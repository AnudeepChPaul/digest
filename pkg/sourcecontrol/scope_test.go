package sourcecontrol

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/AnudeepChPaul/digest/pkg/config"
	"github.com/AnudeepChPaul/digest/pkg/review"
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

func TestRepoScopesAreReusedUntilTheCacheExpires(t *testing.T) {
	repo := gitRepoWithOrigin(t, filepath.Join(t.TempDir(), "service"), "git@git.example.com:team/service.git")
	cfg := &config.Config{GitRepositoryRoots: []string{repo}}
	if scopes := RepoScopes(cfg); !reflect.DeepEqual(scopes, map[string][]string{"git.example.com": {"team/service"}}) {
		t.Fatalf("scopes = %v", scopes)
	}
	if err := exec.Command("git", "-C", repo, "remote", "set-url", "origin", "git@other.example.com:team/service.git").Run(); err != nil {
		t.Fatal(err)
	}
	if _, moved := RepoScopes(cfg)["other.example.com"]; moved {
		t.Error("scopes should be reused within the cache lifetime")
	}
	originalTTL := repoCacheTTL
	repoCacheTTL = 0
	t.Cleanup(func() { repoCacheTTL = originalTTL })
	if _, moved := RepoScopes(cfg)["other.example.com"]; !moved {
		t.Error("an expired cache should read the remotes again")
	}
}
