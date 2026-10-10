# digest

Digest your day-to-day work without acidity. A terminal dashboard for macOS that keeps your notes, pull requests, reviews and git activity in one place, with reminders and weekly brag summaries.

## Install

Homebrew:

```sh
brew tap achandrapaul/tap
brew install digest
```

If Homebrew's `nss` is installed it already provides a `digest` command, so link this one over it:

```sh
brew link --overwrite digest
```

Go:

```sh
go install github.com/achandrapaul/digest/cmd/digest@latest
```

Prebuilt macOS binaries for Apple Silicon and Intel are on the [releases page](https://github.com/achandrapaul/digest/releases).

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
digest --help     # every command and flag (also -h or digest help)
```

Inside the dashboard, `,` opens settings and the footer lists the keys for the current screen.

## Migrate

Quit the dashboard and run this after every version update to keep your notes safe:

```sh
digest migrate
```

`digest migrate --dry-run` changes nothing and prints what it would do.

Notes are named after their id and status: `<id>.md` while active, `<id>-<finished>.done.md` once done and `<id>-<archived>.archived.md` once archived, so the dashboard only opens the files it shows. Migrate renames older notes, gives old and PR notes timestamp ids, and moves their reminders with them. It also moves the old top-level `retention_days` and `janitor_patterns` into the janitor job's `options:`, removes the commands of built-in jobs (their `--root` values become the `roots` option), adds `automation_type` to built-in automations that lack it, and adds the google calendar automation when it's missing. Running it again changes nothing.

Digest locks every file it writes under the digest root (the macOS `uchg` flag), so other programs can read them but can't edit, rename or delete them; digest unlocks a file only while it changes it. `reviews/`, `logs/`, `*.log` files and `config.yaml` stay unlocked because other tools use them. Migrate locks existing files and creates `.app.state.json` (your first note's date and your streak) if it doesn't exist yet, so the brag list and header are right from the first frame. To unlock by hand: `chflags -R nouchg ~/digest`.

## Built-in jobs

`branch-reaper`, `janitor` and `repo-sync` are built into digest, commands and dry-run commands included; `config.yaml` never holds their commands. Without a `jobs:` section, or with a job left out of it, digest runs them with its defaults. To change one, list it under `jobs:` with its name and `options:` only:

```yaml
jobs:
  - name: branch-reaper
    options:
      grace_days: 7 # keeps branches whose PR merged within the last 7 days
      roots: [~/Projects]
  - name: janitor
    options:
      grace_days: 14 # purges quarantine batches older than 14 days
      roots: ["~", ~/Projects]
      patterns:
        - hs_err_pid*.log
        - "*.hprof"
  - name: repo-sync
    options:
      roots: [~/Projects]
```

Every option reaches the job as a `DIGEST_OPTION_<NAME>` environment variable, with lists comma-joined, whether it runs from the dashboard or the shell; built-in jobs ignore options they don't know. `roots` defaults to `~/Projects` for `branch-reaper` and `repo-sync` and `~` for `janitor`, and `--root` on the command line overrides it. Quote a lone `~`, since YAML reads a bare `~` as empty. Putting `command` or `dry-run-command` on a built-in job is a config error; `digest migrate` removes them and turns their `--root` values into the `roots` option. Jobs with any other name are your own and keep their `command` and optional `dry-run-command`.

The janitor's `grace_days` and `patterns` replace the old top-level `retention_days` and `janitor_patterns`; `digest migrate` moves them for you. Add `no_quarantine: true` to the janitor's options (or pass `--no-quarantine`) to delete matching files instead of quarantining them.

On the dashboard they sit under Jobs below today's notes. Select one and press `r` to run it after a confirm, `d` to dry-run it after a confirm (only when it has a `dry-run-command`), or Enter to open its log. Pressing `r` during a dry run asks to stop the dry run and run the job; pressing `d` on a running job asks before stopping it. A job keeps running after you quit digest, and the row shows `done` after a passing dry run and `act` otherwise.

The built-in jobs also run from the shell. `--root` can be repeated and overrides the `roots` option, `--dry-run` changes nothing, and each exits 1 when something needs your action. A `--root` that doesn't exist is an error for `janitor` and `branch-reaper`; `repo-sync` reports it, syncs the other roots and fails only when no root is usable:

```sh
digest repo-sync [--root ~/Projects] [--dry-run]        # fast-forwards clean repos to their upstream
digest janitor [--root ~] [--dry-run] [--no-quarantine] # quarantines (or deletes) crash dumps, removes stale review clones
digest branch-reaper [--root ~/Projects] [--dry-run]    # deletes local branches whose PRs merged
```

## Automations

Notes can be turned into a Jira ticket, a Confluence page, a Google Doc or a Google Calendar event from the action menu (`@`). Each automation in `config.yaml` has an `automation_type` (`ticket`, `confluence`, `doc`, `event`; pr review is `reviewed`), the Claude plugins it may use, `match` phrases that offer it, and a `draft_prompt` and `create_prompt`: digest drafts first, you review the draft, then it creates. A custom automation needs its own `automation_type` and both prompts. `digest init` writes the default config with every built-in automation and its full prompts; `digest init --force` backs up an existing `config.yaml` to `config.yaml.bak` first. `digest init --automations` backs up `config.yaml` and replaces only its `automations:` section with the defaults and their full prompts, keeping your roots, jobs and settings; custom automations are dropped, so copy them back from `config.yaml.bak`.

The google calendar automation uses the claude.ai Google Calendar connector; authenticate it once with `/mcp` in Claude Code.

## Requirements

macOS, git and the GitHub CLI (`gh`). Optional: tmux, Neovim, terminal-notifier and Claude Code for AI reviews and brag summaries.

## Performance

Benchmarks for every release are in [benchmark.md](docs/benchmark/benchmark.md); how releases and benchmarks are recorded is in [release.md](docs/release.md).

## Licence

[MIT](LICENSE)
