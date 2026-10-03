package sourcecontrol

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
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

var queryPattern = regexp.MustCompile(`q='([^']*)'`)

func TestScopeChunksKeepEverySearchUnderTheQueryLimit(t *testing.T) {
	command := "gh api --hostname {host} -X GET search/issues -f q='is:pr is:open draft:false reviewed-by:@me -review-requested:@me {repos}' --jq '.items[].html_url'"
	var fullNames []string
	for index := range 30 {
		fullNames = append(fullNames, fmt.Sprintf("some-organisation/repository-number-%02d", index))
	}

	chunks := scopeChunks(command, fullNames)
	if len(chunks) < 2 {
		t.Fatalf("expected several chunks, got %d", len(chunks))
	}
	var covered []string
	for _, chunk := range chunks {
		query := queryPattern.FindStringSubmatch(strings.ReplaceAll(command, reposPlaceholder, chunk))[1]
		if len(query) > searchQueryLimit {
			t.Errorf("query is %d chars: %q", len(query), query)
		}
		for _, qualifier := range strings.Fields(chunk) {
			covered = append(covered, strings.TrimPrefix(qualifier, "repo:"))
		}
	}
	if !reflect.DeepEqual(covered, fullNames) {
		t.Errorf("chunks cover %v, want %v", covered, fullNames)
	}
}

func TestHostQueries(t *testing.T) {
	scoped := "gh api -f q='is:pr {repos}'"
	scopes := map[string][]string{"git.example.com": {"team/service"}}

	if got := hostQueries(scoped, scopes, "git.example.com"); !reflect.DeepEqual(got, []string{"repo:team/service"}) {
		t.Errorf("scoped host queries = %v", got)
	}
	if got := hostQueries(scoped, scopes, "other.example.com"); len(got) != 0 {
		t.Errorf("host without configured repos should be skipped, got %v", got)
	}
	if got := hostQueries(scoped, nil, "other.example.com"); !reflect.DeepEqual(got, []string{""}) {
		t.Errorf("no scoping should run once unscoped, got %v", got)
	}
	if got := hostQueries("gh api -f q='is:pr'", scopes, "other.example.com"); !reflect.DeepEqual(got, []string{""}) {
		t.Errorf("command without {repos} should run unchanged, got %v", got)
	}
}

func TestUnloadedHosts(t *testing.T) {
	loadedRef, missingRef := prRefOn("git.example.com", 1), prRefOn("other.example.com", 2)
	hosts := unloadedHosts([]review.PRRef{loadedRef, missingRef}, []review.QueuedPR{{Ref: loadedRef}})
	if !reflect.DeepEqual(hosts, []string{"other.example.com"}) {
		t.Errorf("unloaded hosts = %v", hosts)
	}
}

func prRefOn(host string, number int) review.PRRef {
	return review.PRRef{Host: host, Owner: "team", Repo: "service", Number: number, URL: fmt.Sprintf("https://%s/team/service/pull/%d", host, number)}
}
