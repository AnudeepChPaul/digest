package review

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type Event string

const (
	EventApprove        Event = "APPROVE"
	EventRequestChanges Event = "REQUEST_CHANGES"
	EventComment        Event = "COMMENT"
)

var (
	ErrRejectNeedsComment = errors.New("request changes needs at least one selected comment or a written comment")
	ErrNothingToPost      = errors.New("select at least one comment to post")
)

type ReviewComment struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Side string `json:"side"`
	Body string `json:"body"`
}

type Payload struct {
	CommitID string          `json:"commit_id,omitempty"`
	Body     string          `json:"body,omitempty"`
	Event    Event           `json:"event"`
	Comments []ReviewComment `json:"comments,omitempty"`
}

func commentBody(finding Finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**[%s] %s**", finding.Severity, finding.Title)
	if finding.Body != "" {
		b.WriteString("\n\n" + finding.Body)
	}
	if finding.Suggestion != "" {
		b.WriteString("\n\n**Suggestion:** " + finding.Suggestion)
	}
	return b.String()
}

func BuildPayload(event Event, selected []Finding, body, headSHA string) (Payload, error) {
	body = strings.TrimSpace(body)
	payload := Payload{CommitID: headSHA, Event: event}
	var general []string
	if body != "" {
		general = append(general, body)
	}
	for _, finding := range selected {
		if finding.Path == "" || finding.Line <= 0 {
			general = append(general, commentBody(finding))
			continue
		}
		payload.Comments = append(payload.Comments, ReviewComment{Path: finding.Path, Line: finding.Line, Side: "RIGHT", Body: commentBody(finding)})
	}
	payload.Body = strings.Join(general, "\n\n---\n\n")

	hasContent := payload.Body != "" || len(payload.Comments) > 0
	switch event {
	case EventComment:
		if !hasContent {
			return Payload{}, ErrNothingToPost
		}
	case EventRequestChanges:
		if !hasContent {
			return Payload{}, ErrRejectNeedsComment
		}
	}
	return payload, nil
}

func FoldIntoBody(payload Payload) Payload {
	folded := payload
	folded.Comments = nil
	parts := []string{}
	if payload.Body != "" {
		parts = append(parts, payload.Body)
	}
	for _, comment := range payload.Comments {
		parts = append(parts, fmt.Sprintf("`%s:%d`\n\n%s", comment.Path, comment.Line, comment.Body))
	}
	folded.Body = strings.Join(parts, "\n\n---\n\n")
	return folded
}

var postReview = func(ctx context.Context, ref PRRef, body []byte) error {
	args := []string{"api", "--method", "POST", fmt.Sprintf("repos/%s/pulls/%d/reviews", ref.NameWithOwner(), ref.Number), "--input", "-"}
	if ref.Host != "" {
		args = append(args, "--hostname", ref.Host)
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.WaitDelay = commandWaitDelay
	cmd.Stdin = bytes.NewReader(body)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gh api: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

func Submit(ctx context.Context, ref PRRef, payload Payload) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	err = postReview(ctx, ref, encoded)
	if err == nil || len(payload.Comments) == 0 || !strings.Contains(err.Error(), "422") {
		return err
	}
	encoded, marshalErr := json.Marshal(FoldIntoBody(payload))
	if marshalErr != nil {
		return marshalErr
	}
	if retryErr := postReview(ctx, ref, encoded); retryErr != nil {
		return fmt.Errorf("%v; retry without inline comments: %w", err, retryErr)
	}
	return nil
}
