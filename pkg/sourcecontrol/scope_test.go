package sourcecontrol

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"app/pkg/config"
	"app/pkg/review"
)

func TestParseRemoteURL(t *testing.T) {
	cases := map[string][2]string{
		"ssh://git@git.example.com/team/service.git":  {"git.example.com", "team/service"},
		"ssh://git@git.example.com:2222/team/service": {"git.example.com", "team/service"},
		"git@other.example.com:org/app.git":           {"other.example.com", "org/app"},
		"https://git.example.com/team/service.git":    {"git.example.com", "team/service"},
		"https://user@git.example.com/team/service/":  {"git.example.com", "team/service"},
	}
	for remote, want := range cases {
		host, fullName, ok := parseRemoteURL(remote)
		if !ok || host != want[0] || fullName != want[1] {
			t.Errorf("parseRemoteURL(%q) = %q %q %v, want %v", remote, host, fullName, ok, want)
		}
	}
	for _, remote := range []string{"", "/local/path/repo", "not a remote"} {
		if _, _, ok := parseRemoteURL(remote); ok {
			t.Errorf("parseRemoteURL(%q) should fail", remote)
		}
	}
}

func gitRepoWithOrigin(t *testing.T, path, origin string) string {
	t.Helper()
	for _, args := range [][]string{{"init", "-q", path}, {"-C", path, "remote", "add", "origin", origin}} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, output)
		}
	}
	return path
}

func TestRepoScopesGroupsConfiguredReposByHost(t *testing.T) {
	base := t.TempDir()
	first := gitRepoWithOrigin(t, filepath.Join(base, "service"), "ssh://git@git.example.com/team/service.git")
	second := gitRepoWithOrigin(t, filepath.Join(base, "app"), "git@other.example.com:org/app.git")
	third := gitRepoWithOrigin(t, filepath.Join(base, "tool"), "https://git.example.com/team/tool.git")
	makeRepo(t, filepath.Join(base, "no-origin"))
	cfg := &config.Config{GitRepositoryRoots: []string{first, second, third, filepath.Join(base, "no-origin")}}

	scopes := RepoScopes(cfg)
	for host := range scopes {
		sort.Strings(scopes[host])
	}
	want := map[string][]string{
		"git.example.com":   {"team/service", "team/tool"},
		"other.example.com": {"org/app"},
	}
	if !reflect.DeepEqual(scopes, want) {
		t.Errorf("scopes = %v, want %v", scopes, want)
	}
	if RepoScopes(&config.Config{}) != nil {
		t.Errorf("no repo roots should mean no scoping")
	}
}

func prRefOn(host string, number int) review.PRRef {
	return review.PRRef{Host: host, Owner: "team", Repo: "service", Number: number, URL: fmt.Sprintf("https://%s/team/service/pull/%d", host, number)}
}
