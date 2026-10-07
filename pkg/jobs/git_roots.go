package jobs

import (
	"errors"
	"fmt"
	"strings"
)

func CheckGitRoots(gitEnabled bool, roots []string) error {
	if !gitEnabled {
		return nil
	}
	if len(roots) == 0 {
		return errors.New("show_git is on but git_repository_roots is empty; add a folder that holds git repos or set show_git: false")
	}
	if repos, _ := DiscoverRepos(roots); len(repos) == 0 {
		return fmt.Errorf("no git repos found under %s; point git_repository_roots at a folder with git repos or set show_git: false", strings.Join(roots, ", "))
	}
	return nil
}
