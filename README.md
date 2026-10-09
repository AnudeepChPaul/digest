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

## Migrate

Quit the dashboard and run this after every version update to keep your notes safe:

```sh
digest migrate
```

Notes are named after their id and status: `<id>.md` while active, `<id>-<finished>.done.md` once done and `<id>-<archived>.archived.md` once archived, so the dashboard only opens the files it shows. Migrate renames older notes, gives old and PR notes timestamp ids, and moves their reminders with them. Running it again changes nothing.

Digest locks every file it writes under the digest root (the macOS `uchg` flag), so other programs can read them but can't edit, rename or delete them; digest unlocks a file only while it changes it. `reviews/`, `logs/`, `*.log` files and `config.yaml` stay unlocked because other tools use them. Migrate locks existing files and creates `.app.state.json` (your first note's date and your streak) if it doesn't exist yet, so the brag list and header are right from the first frame. To unlock by hand: `chflags -R nouchg ~/digest`.

## Pre-defined jobs

Jobs are shell commands listed under `jobs:` in `config.yaml`, each with a `command` and an optional `dry-run-command`. `branch-reaper` and `janitor` come by default:

```yaml
jobs:
  - name: branch-reaper
    dry-run-command: "digest branch-reaper --root ~/Projects --dry-run"
    command: "digest branch-reaper --root ~/Projects"
```

On the dashboard they sit under Jobs below today's notes. Select one and press `r` to run it after a confirm, `d` to dry-run it, or Enter to open its log. Pressing `d` on a running job asks before stopping it. A job keeps running after you quit digest, and the row shows `done` after a passing dry run and `act` otherwise.

The built-in jobs also run from the shell. `--root` can be repeated, `--dry-run` changes nothing, and each exits 1 when something needs your action:

```sh
digest repo-sync --root ~/Projects [--dry-run]       # fast-forwards clean repos to their upstream
digest janitor --root ~ [--dry-run]                  # quarantines crash dumps, removes stale review clones
digest branch-reaper --root ~/Projects [--dry-run]   # deletes local branches whose PRs merged
```

## Requirements

macOS, git and the GitHub CLI (`gh`). Optional: tmux, Neovim, terminal-notifier and Claude Code for AI reviews and brag summaries.

## Performance

Benchmarks for every release are in [benchmark.md](docs/benchmark/benchmark.md); how releases and benchmarks are recorded is in [release.md](docs/release.md).

## Licence

[MIT](LICENSE)
