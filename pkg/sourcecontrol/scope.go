package sourcecontrol

import (
	"context"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/AnudeepChPaul/digest/pkg/config"
)

var scpRemotePattern = regexp.MustCompile(`^[^@/\s]+@([^:/\s]+):([^\s]+)$`)

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
	roots := cfg.GitRepositoryRoots
	return repoScopes.get(rootsKey(roots), func() map[string][]string { return readRepoScopes(roots) })
}

var remoteLookupTimeout = 5 * time.Second

func originRemote(repoPath string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), remoteLookupTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "remote", "get-url", "origin")
	cmd.WaitDelay = commandWaitDelay
	return cmd.Output()
}

func readRepoScopes(roots []string) map[string][]string {
	scopes := map[string][]string{}
	seen := map[string]bool{}
	for _, repoPath := range discoverReposCached(roots) {
		remote, err := originRemote(repoPath)
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
