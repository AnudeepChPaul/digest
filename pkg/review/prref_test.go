package review

import "testing"

func TestParsePRURL(t *testing.T) {
	cases := []struct {
		raw  string
		want PRRef
	}{
		{"https://github.com/acme/console/pull/19162", PRRef{Host: "github.com", Owner: "acme", Repo: "console", Number: 19162, URL: "https://github.com/acme/console/pull/19162"}},
		{"https://git.example.com/team/monkey/pull/7/files", PRRef{Host: "git.example.com", Owner: "team", Repo: "monkey", Number: 7, URL: "https://git.example.com/team/monkey/pull/7"}},
	}
	for _, c := range cases {
		got, err := ParsePRURL(c.raw)
		if err != nil {
			t.Fatalf("%s: %v", c.raw, err)
		}
		if got != c.want {
			t.Errorf("%s: got %+v want %+v", c.raw, got, c.want)
		}
	}
}

func TestParsePRURLRejectsInvalid(t *testing.T) {
	for _, raw := range []string{"", "https://github.com/a/b", "https://github.com/a/b/pull/x", "ftp://github.com/a/b/pull/1", "https://github.com/a/b/issues/1"} {
		if _, err := ParsePRURL(raw); err == nil {
			t.Errorf("%q: expected error", raw)
		}
	}
}

func TestPRRefNames(t *testing.T) {
	ref := PRRef{Host: "github.com", Owner: "o", Repo: "console", Number: 12}
	if ref.DirName() != "console_12" {
		t.Errorf("DirName = %q", ref.DirName())
	}
	if ref.NameWithOwner() != "o/console" {
		t.Errorf("NameWithOwner = %q", ref.NameWithOwner())
	}
	if ref.CloneSpec() != "github.com/o/console" {
		t.Errorf("CloneSpec = %q", ref.CloneSpec())
	}
	ghe := PRRef{Host: "git.example.com", Owner: "team", Repo: "monkey", Number: 1}
	if ghe.CloneSpec() != "git.example.com/team/monkey" {
		t.Errorf("CloneSpec = %q", ghe.CloneSpec())
	}
}
