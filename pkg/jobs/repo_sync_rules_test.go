package jobs

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
)

func TestRepoSyncTreatsARepoWithoutRemotesAsNoRemote(t *testing.T) {
	syncRoot := t.TempDir()
	local := filepath.Join(syncRoot, "local")
	runGit(t, syncRoot, "init", "--quiet", "-b", "main", local)
	commitFile(t, local, "README", "hi\n")

	if info := ClassifyRepo(local, false); info.State != NoRemote {
		t.Errorf("state = %s, want %s", info.State, NoRemote)
	}
	job := &RepoSyncJob{Roots: []string{syncRoot}, Logger: log.New(io.Discard)}
	result, err := job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Degraded || !slices.Contains(result.Drafts, "local: no-remote") {
		t.Errorf("result = %+v, want no-remote without degrading", result)
	}
}

func TestRepoSyncAsksTheRemoteForItsDefaultBranchWhenOriginHeadIsUnset(t *testing.T) {
	base := t.TempDir()
	origin, seed := filepath.Join(base, "origin.git"), filepath.Join(base, "seed")
	runGit(t, base, "init", "--quiet", "--bare", "-b", "trunk", origin)
	runGit(t, base, "clone", "--quiet", origin, seed)
	commitFile(t, seed, "README", "hi\n")
	runGit(t, seed, "push", "--quiet", "origin", "trunk", "trunk:main")
	_, clone := cloneUnder(t, origin)
	runGit(t, clone, "config", "remote.origin.followRemoteHEAD", "never")
	runGit(t, clone, "remote", "set-head", "origin", "--delete")

	if info := ClassifyRepo(clone, true); info.State != UpToDate || info.Branch != "trunk" {
		t.Errorf("info = %+v, want trunk up to date", info)
	}
	runGit(t, clone, "checkout", "--quiet", "-b", "main", "origin/main")
	if info := ClassifyRepo(clone, true); info.State != OffDefault || info.Detail != "on main, default is trunk" {
		t.Errorf("info = %+v, want main reported off the remote's default trunk", info)
	}
}

func repoWithUnbornRemoteHead(t *testing.T, branches ...string) string {
	t.Helper()
	base := t.TempDir()
	origin, seed := filepath.Join(base, "origin.git"), filepath.Join(base, "seed")
	runGit(t, base, "init", "--quiet", "--bare", "-b", branches[0], origin)
	runGit(t, base, "clone", "--quiet", origin, seed)
	commitFile(t, seed, "README", "hi\n")
	for _, branch := range branches {
		runGit(t, seed, "push", "--quiet", "origin", "HEAD:"+branch)
	}
	runGit(t, origin, "symbolic-ref", "HEAD", "refs/heads/unborn")
	_, clone := cloneUnder(t, origin)
	return clone
}

func TestRepoSyncFallsBackToMainThenMaster(t *testing.T) {
	withMaster := repoWithUnbornRemoteHead(t, "master")
	runGit(t, withMaster, "checkout", "--quiet", "-b", "master", "origin/master")
	if info := ClassifyRepo(withMaster, true); info.State != UpToDate || info.Branch != "master" {
		t.Errorf("master only: info = %+v, want master as the default", info)
	}

	withNeither := repoWithUnbornRemoteHead(t, "trunk")
	runGit(t, withNeither, "checkout", "--quiet", "-b", "trunk", "origin/trunk")
	if info := ClassifyRepo(withNeither, true); info.State != OffDefault || info.Detail != "on trunk, default is main" {
		t.Errorf("neither: info = %+v, want main assumed", info)
	}

	withBoth := repoWithUnbornRemoteHead(t, "master", "main")
	runGit(t, withBoth, "checkout", "--quiet", "-b", "master", "origin/master")
	if info := ClassifyRepo(withBoth, true); info.State != OffDefault || info.Detail != "on master, default is main" {
		t.Errorf("main and master: info = %+v, want main preferred", info)
	}
}

func behindClone(t *testing.T, incoming string) (syncRoot, clone string) {
	t.Helper()
	origin, seed := originWithSeed(t)
	syncRoot, clone = cloneUnder(t, origin)
	commitFile(t, seed, incoming, "from upstream\n")
	runGit(t, seed, "push", "--quiet", "origin", "main")
	return syncRoot, clone
}

func TestRepoSyncFastForwardsPastUntrackedFiles(t *testing.T) {
	syncRoot, clone := behindClone(t, "CHANGES")
	if err := os.WriteFile(filepath.Join(clone, "scratch.txt"), []byte("mine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	job := &RepoSyncJob{Roots: []string{syncRoot}, Logger: log.New(io.Discard)}
	result, err := job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(result.ActionsTaken, "clone: fast-forwarded") {
		t.Errorf("result = %+v, want untracked files to leave the fast-forward alone", result)
	}
}

func TestRepoSyncReportsAFastForwardRefusedByAClashingUntrackedFile(t *testing.T) {
	syncRoot, clone := behindClone(t, "CHANGES")
	if err := os.WriteFile(filepath.Join(clone, "CHANGES"), []byte("mine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	job := &RepoSyncJob{Roots: []string{syncRoot}, Logger: log.New(io.Discard)}
	result, err := job.Run(false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(result.Drafts, "clone: fast-forward refused") || !result.Degraded {
		t.Errorf("result = %+v, want a refused fast-forward", result)
	}
	if content, _ := os.ReadFile(filepath.Join(clone, "CHANGES")); string(content) != "mine\n" {
		t.Errorf("the untracked file was overwritten: %q", content)
	}
}

func TestRepoSyncDryRunListsTheFastForward(t *testing.T) {
	syncRoot, _ := behindClone(t, "CHANGES")
	needsAction, err := RunRepoSync([]string{syncRoot}, true)
	if err != nil || !needsAction {
		t.Errorf("needsAction = %v, err = %v; a dry run with work to do needs action", needsAction, err)
	}
}

func TestClassifyRepoStates(t *testing.T) {
	origin, seed := originWithSeed(t)
	_, clone := cloneUnder(t, origin)

	if err := os.WriteFile(filepath.Join(clone, "README"), []byte("edited\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if info := ClassifyRepo(clone, true); info.State != Dirty {
		t.Errorf("tracked change: %+v, want dirty", info)
	}
	runGit(t, clone, "checkout", "--quiet", "--", "README")

	commitFile(t, clone, "LOCAL", "mine\n")
	if info := ClassifyRepo(clone, true); info.State != Ahead || info.Detail != "1 ahead" {
		t.Errorf("local commit: %+v, want 1 ahead", info)
	}
	commitFile(t, seed, "REMOTE", "theirs\n")
	runGit(t, seed, "push", "--quiet", "origin", "main")
	if info := ClassifyRepo(clone, false); info.State != Diverged || info.Detail != "1 ahead, 1 behind" {
		t.Errorf("both sides moved: %+v, want diverged", info)
	}

	runGit(t, clone, "checkout", "--quiet", "--detach")
	if info := ClassifyRepo(clone, true); info.State != Detached {
		t.Errorf("detached HEAD: %+v", info)
	}
	runGit(t, clone, "checkout", "--quiet", "main")
	runGit(t, clone, "branch", "--quiet", "--unset-upstream")
	if info := ClassifyRepo(clone, true); info.State != NoUpstream {
		t.Errorf("no upstream: %+v", info)
	}

	runGit(t, clone, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
	if info := ClassifyRepo(clone, true); info.State != Unreachable {
		t.Errorf("missing origin: %+v, want unreachable", info)
	}
}

func TestRepoSyncSummarisesEveryStateItSees(t *testing.T) {
	origin, _ := originWithSeed(t)
	syncRoot := t.TempDir()
	for _, name := range []string{"clean", "ahead", "gone"} {
		runGit(t, syncRoot, "clone", "--quiet", origin, filepath.Join(syncRoot, name))
	}
	commitFile(t, filepath.Join(syncRoot, "ahead"), "LOCAL", "mine\n")
	runGit(t, filepath.Join(syncRoot, "gone"), "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))

	job := &RepoSyncJob{Roots: []string{syncRoot}, Workers: 2, Logger: log.New(io.Discard)}
	result, err := job.Run(true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ahead: ahead", "gone: remote unreachable"} {
		if !slices.Contains(result.Drafts, want) {
			t.Errorf("drafts = %v, want %q", result.Drafts, want)
		}
	}
	if !result.Degraded || result.Summary != "3 repositories · 0 to fast-forward · 2 need attention" {
		t.Errorf("result = %+v", result)
	}
}

func TestRepoSyncWithNoReposSaysSo(t *testing.T) {
	job := &RepoSyncJob{Roots: []string{t.TempDir()}}
	result, err := job.Run(true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != "0 repositories discovered" || result.Changed {
		t.Errorf("result = %+v", result)
	}
}

func TestRepoSyncWithNoUsableRootIsAnError(t *testing.T) {
	if _, err := RunRepoSync([]string{filepath.Join(t.TempDir(), "nope")}, false); err == nil {
		t.Error("a missing only --root should be an error")
	}
	if _, err := RunRepoSync([]string{filepath.Join(t.TempDir(), "nope"), "$DIGEST_TEST_UNSET_VARIABLE/projects"}, false); err == nil {
		t.Error("no usable --root at all should be an error")
	}
	unreadable := t.TempDir()
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0755) })
	if _, err := DiscoverRepos([]string{unreadable}); err == nil {
		t.Error("an unreadable root should be an error")
	}
	if _, err := DiscoverRepos([]string{"$DIGEST_TEST_UNSET_VARIABLE/projects"}); err == nil {
		t.Error("an unexpandable root should be an error")
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverRepos([]string{file}); err == nil {
		t.Error("a root that is a file should be an error")
	}
}

func TestDiscoverReposKeepsReposFromValidRootsAlongsideTheError(t *testing.T) {
	withRepo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(withRepo, "app", ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(withRepo, ".hidden", "skipped", ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(withRepo, "notes.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	locked := filepath.Join(withRepo, "locked")
	if err := os.Mkdir(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0755) })
	repos, err := DiscoverRepos([]string{filepath.Join(t.TempDir(), "nope"), withRepo})
	if err == nil {
		t.Error("the missing root should be reported")
	}
	if !slices.Equal(repos, []string{filepath.Join(withRepo, "app")}) {
		t.Errorf("repos = %v, want only app", repos)
	}
}

func TestRepoSyncReportsAnUnusableRootAndSyncsTheOthers(t *testing.T) {
	syncRoot := t.TempDir()
	local := filepath.Join(syncRoot, "local")
	runGit(t, syncRoot, "init", "--quiet", "-b", "main", local)
	commitFile(t, local, "README", "hi\n")
	missing := filepath.Join(t.TempDir(), "nope")

	job := &RepoSyncJob{Roots: []string{missing, syncRoot}, Logger: log.New(io.Discard)}
	result, err := job.Run(false)
	if err != nil {
		t.Fatalf("one usable root should keep the job going: %v", err)
	}
	if !slices.Contains(result.Drafts, "local: no-remote") {
		t.Errorf("drafts = %v, want the repo under the usable root synced", result.Drafts)
	}
	if !result.Degraded || !strings.Contains(strings.Join(result.Drafts, "\n"), "repository root "+missing) || !strings.Contains(result.Summary, "1 root unusable") {
		t.Errorf("result = %+v, want the missing root reported", result)
	}

	emptyRoot := t.TempDir()
	job = &RepoSyncJob{Roots: []string{missing, emptyRoot}, Logger: log.New(io.Discard)}
	if result, err = job.Run(true); err != nil || !result.Degraded || !strings.Contains(result.Details, "repository root "+missing) {
		t.Errorf("an empty usable root: result = %+v, err = %v, want the missing root reported", result, err)
	}
}
