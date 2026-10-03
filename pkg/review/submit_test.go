package review

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

var sampleFindings = []Finding{
	{Severity: "high", Title: "Null deref", Path: "a.ts", Line: 4, Body: "x may be nil", Suggestion: "guard it"},
	{Severity: "critical", Title: "Design", Body: "no file anchor"},
}

func TestBuildPayloadComment(t *testing.T) {
	p, err := BuildPayload(EventComment, sampleFindings, "", "sha1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Event != EventComment || p.CommitID != "sha1" || len(p.Comments) != 1 {
		t.Fatalf("payload = %+v", p)
	}
	c := p.Comments[0]
	if c.Path != "a.ts" || c.Line != 4 || c.Side != "RIGHT" || !strings.Contains(c.Body, "Null deref") || !strings.Contains(c.Body, "guard it") {
		t.Errorf("comment = %+v", c)
	}
	if !strings.Contains(p.Body, "Design") {
		t.Errorf("unanchored finding should be in body: %q", p.Body)
	}
}

func TestBuildPayloadValidation(t *testing.T) {
	if _, err := BuildPayload(EventComment, nil, "", "s"); err == nil {
		t.Errorf("comment without findings should fail")
	}
	if _, err := BuildPayload(EventRequestChanges, nil, "  ", "s"); !errors.Is(err, ErrRejectNeedsComment) {
		t.Errorf("reject without comment: err = %v", err)
	}
	if p, err := BuildPayload(EventRequestChanges, nil, "please fix", "s"); err != nil || p.Body != "please fix" {
		t.Errorf("reject with body: %+v %v", p, err)
	}
	if p, err := BuildPayload(EventApprove, nil, "", "s"); err != nil || p.Event != EventApprove {
		t.Errorf("plain approve: %+v %v", p, err)
	}
	if p, err := BuildPayload(EventApprove, sampleFindings[:1], "", "s"); err != nil || len(p.Comments) != 1 {
		t.Errorf("approve with comments: %+v %v", p, err)
	}
}

func TestFoldIntoBody(t *testing.T) {
	p, _ := BuildPayload(EventComment, sampleFindings, "", "s")
	folded := FoldIntoBody(p)
	if len(folded.Comments) != 0 || !strings.Contains(folded.Body, "a.ts:4") || !strings.Contains(folded.Body, "Design") {
		t.Errorf("folded = %+v", folded)
	}
}

func TestSubmitRetriesFoldedOnUnprocessable(t *testing.T) {
	var bodies []Payload
	original := postReview
	postReview = func(ctx context.Context, ref PRRef, body []byte) error {
		var p Payload
		_ = json.Unmarshal(body, &p)
		bodies = append(bodies, p)
		if len(bodies) == 1 {
			return errors.New("HTTP 422: Unprocessable Entity (Line could not be resolved)")
		}
		return nil
	}
	defer func() { postReview = original }()

	p, _ := BuildPayload(EventComment, sampleFindings, "", "s")
	if err := Submit(context.Background(), PRRef{Owner: "o", Repo: "r", Number: 1}, p); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || len(bodies[1].Comments) != 0 {
		t.Errorf("bodies = %+v", bodies)
	}
}
