package sourcecontrol

import (
	"net/url"
	"os/exec"
	"regexp"
	"strings"

	"app/pkg/config"
	"app/pkg/jobs"
)

const (
	reposPlaceholder     = "{repos}"
	searchQueryLimit     = 256
	placeholderExpansion = 16
)

var (
	scpRemotePattern   = regexp.MustCompile(`^[^@/\s]+@([^:/\s]+):([^\s]+)$`)
	searchQueryPattern = regexp.MustCompile(`q='([^']*)'`)
)

func parseRemoteURL(remote string) (host, fullName string, ok bool) {
	remote = strings.TrimSpace(remote)
	var path string
	if match := scpRemotePattern.FindStringSubmatch(remote); match != nil {
		host, path = match[1], match[2]
	} else {
		parsed, err := url.Parse(remote)
		if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
			return "", "", false
		}
		host, path = parsed.Hostname(), parsed.Path
	}
	segments := strings.Split(strings.Trim(strings.TrimSuffix(strings.Trim(path, "/"), ".git"), "/"), "/")
	if len(segments) != 2 || segments[0] == "" || segments[1] == "" {
		return "", "", false
	}
	return host, segments[0] + "/" + segments[1], true
}

func RepoScopes(cfg *config.Config) map[string][]string {
	if cfg == nil || len(cfg.GitRepositoryRoots) == 0 {
		return nil
	}
	repoPaths, _ := jobs.DiscoverRepos(cfg.GitRepositoryRoots)
	scopes := map[string][]string{}
	seen := map[string]bool{}
	for _, repoPath := range repoPaths {
		remote, err := exec.Command("git", "-C", repoPath, "remote", "get-url", "origin").Output()
		if err != nil {
			continue
		}
		host, fullName, ok := parseRemoteURL(string(remote))
		if !ok || seen[host+"/"+fullName] {
			continue
		}
		seen[host+"/"+fullName] = true
		scopes[host] = append(scopes[host], fullName)
	}
	return scopes
}

func scopeChunks(command string, fullNames []string) []string {
	baseQuery := strings.ReplaceAll(command, reposPlaceholder, "")
	if match := searchQueryPattern.FindStringSubmatch(baseQuery); match != nil {
		baseQuery = match[1]
	}
	budget := searchQueryLimit - placeholderExpansion - len(baseQuery)

	var chunks []string
	var current []string
	currentLength := 0
	for _, fullName := range fullNames {
		qualifier := "repo:" + fullName
		if len(current) > 0 && currentLength+1+len(qualifier) > budget {
			chunks = append(chunks, strings.Join(current, " "))
			current, currentLength = nil, 0
		}
		if len(current) > 0 {
			currentLength++
		}
		current = append(current, qualifier)
		currentLength += len(qualifier)
	}
	if len(current) > 0 {
		chunks = append(chunks, strings.Join(current, " "))
	}
	return chunks
}

func hostQueries(command string, scopes map[string][]string, host string) []string {
	if scopes == nil || !strings.Contains(command, reposPlaceholder) {
		return []string{""}
	}
	return scopeChunks(command, scopes[host])
}
