package review

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type PRRef struct {
	Host   string `json:"host"`
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	URL    string `json:"url"`
}

func ParsePRURL(raw string) (PRRef, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return PRRef{}, fmt.Errorf("not an https pull request URL: %q", raw)
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) < 4 || segments[2] != "pull" {
		return PRRef{}, fmt.Errorf("not a pull request URL: %q", raw)
	}
	number, err := strconv.Atoi(segments[3])
	if err != nil || number <= 0 {
		return PRRef{}, fmt.Errorf("invalid pull request number in %q", raw)
	}
	owner, repo := segments[0], segments[1]
	return PRRef{
		Host:   parsed.Host,
		Owner:  owner,
		Repo:   repo,
		Number: number,
		URL:    fmt.Sprintf("https://%s/%s/%s/pull/%d", parsed.Host, owner, repo, number),
	}, nil
}

func (p PRRef) DirName() string {
	if p.Owner == "" {
		return p.legacyDirName()
	}
	return fmt.Sprintf("%s_%s_%d", p.Owner, p.Repo, p.Number)
}

func (p PRRef) legacyDirName() string {
	return fmt.Sprintf("%s_%d", p.Repo, p.Number)
}

func (p PRRef) NameWithOwner() string {
	return p.Owner + "/" + p.Repo
}

func (p PRRef) CloneSpec() string {
	if p.Host == "" {
		return p.NameWithOwner()
	}
	return p.Host + "/" + p.NameWithOwner()
}
