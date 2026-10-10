package paths_test

import (
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/achandrapaul/digest/pkg/paths"
)

func TestExpandReplacesHomeAndVariables(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DIGEST_TEST_FOLDER", "notes")
	cases := map[string]string{
		"":                            "",
		"~":                           home,
		"~/digest":                    filepath.Join(home, "digest"),
		"$HOME/x":                     filepath.Join(home, "x"),
		"/data/$DIGEST_TEST_FOLDER":   "/data/notes",
		"/data/${DIGEST_TEST_FOLDER}": "/data/notes",
		"/plain/path":                 "/plain/path",
		"relative/~":                  "relative/~",
	}
	for input, want := range cases {
		if got, err := paths.Expand(input); err != nil || got != want {
			t.Errorf("Expand(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestExpandRejectsUnsetVariables(t *testing.T) {
	for _, input := range []string{"/data/$DIGEST_UNSET_VARIABLE/x", "/data/${DIGEST_UNSET_VARIABLE}"} {
		if got, err := paths.Expand(input); err == nil || !strings.Contains(err.Error(), "DIGEST_UNSET_VARIABLE") {
			t.Errorf("Expand(%q) = %q, %v; want an error naming DIGEST_UNSET_VARIABLE", input, got, err)
		}
	}
}

func TestExpandFindsAnotherUsersHome(t *testing.T) {
	current, err := user.Current()
	if err != nil {
		t.Skip(err)
	}
	got, err := paths.Expand("~" + current.Username + "/x")
	if err != nil || got != filepath.Join(current.HomeDir, "x") {
		t.Errorf("Expand(~%s/x) = %q, %v; want %q", current.Username, got, err, filepath.Join(current.HomeDir, "x"))
	}
	if got, err := paths.Expand("~" + current.Username); err != nil || got != current.HomeDir {
		t.Errorf("Expand(~%s) = %q, %v", current.Username, got, err)
	}
}

func TestExpandRejectsAnUnknownUser(t *testing.T) {
	if got, err := paths.Expand("~digest-no-such-user/x"); err == nil || !strings.Contains(err.Error(), "digest-no-such-user") {
		t.Errorf("Expand = %q, %v; want an error naming the user", got, err)
	}
}

func TestExpandFailsWithoutAHome(t *testing.T) {
	t.Setenv("HOME", "")
	if got, err := paths.Expand("~/x"); err == nil {
		t.Errorf("Expand(~/x) = %q, want an error when the home folder is unknown", got)
	}
}
