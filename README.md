# digest

Digest your day-to-day work without acidity. A terminal dashboard for macOS that keeps your notes, pull requests, reviews and git activity in one place, with reminders and weekly brag summaries.

## Install

Homebrew:

```sh
brew tap anudeepchpaul/tap
brew install digest
```

If Homebrew's `nss` is installed it already provides a `digest` command, so link this one over it:

```sh
brew link --overwrite digest
```

Go:

```sh
go install github.com/AnudeepChPaul/digest/cmd/digest@latest
```

Prebuilt macOS binaries for Apple Silicon and Intel are on the [releases page](https://github.com/AnudeepChPaul/digest/releases).

From source, with [mise](https://mise.jdx.dev):

```sh
mise run build
```

## Setup

After installing, run these three commands once.

1. Install what digest depends on:

   ```sh
   digest install
   ```

   This installs any missing tools with Homebrew, the launchd agent that sends reminders, and a keyboard shortcut that opens digest. It asks before each one.

2. Check that digest can run everything:

   ```sh
   digest doctor
   ```

   This lists every tool digest uses, whether it was found and which feature needs it. It exits with an error if a required tool is missing; run `digest install` again to add it.

3. Configure digest:

   ```sh
   digest setup
   ```

   This walks through git roots, work days, summaries, hints and notifications. You can also open it from the dashboard with `,`, and the first run of `digest` starts it on its own.

## Usage

```sh
digest            # open the dashboard
digest --help     # every command and flag
```

Inside the dashboard, `,` opens settings and the footer lists the keys for the current screen.

### Upgrading notes

Notes are named after their id and status: `<id>.md` while active, `<id>-<finished>.done.md` once done and `<id>-<archived>.archived.md` once archived, so the dashboard only opens the files it shows. Quit the TUI and run `digest migrate` once to rename older notes, give old and PR notes timestamp ids, and move their reminders with them. Running it again changes nothing.

## Requirements

macOS, git and the GitHub CLI (`gh`). Optional: tmux, Neovim, terminal-notifier and Claude Code for AI reviews and brag summaries.

## Performance

Every release records how fast digest starts, loads notes, redraws, searches and saves, at 1 to 10k notes. [BENCHMARK.md](docs/benchmark/BENCHMARK.md) has the latest numbers and their history.

`mise run bench` measures locally without recording, and `mise run bench:full` adds 100k notes.

## Releases

digest follows [Semantic Versioning](https://semver.org), and the commit messages since the last release decide the next version, following [Conventional Commits](https://www.conventionalcommits.org):

| Commits since the last release | Release |
|---|---|
| `feat!:`, `fix(scope)!:` or a `BREAKING CHANGE:` line in the body | major |
| `feat:` | minor |
| `fix:` or `perf:` | patch |
| only `docs:`, `ci:`, `chore:`, `test:` and the like | none |

On every push to `main`, GitHub Actions runs vet and tests and works out that level. It then bumps `.version` and adds a section to [CHANGELOG.md](CHANGELOG.md). That commit tags the release, publishes the binaries with the changelog section as release notes, and updates the Homebrew formula. Don't edit `.version` by hand.

## Licence

[MIT](LICENSE)
