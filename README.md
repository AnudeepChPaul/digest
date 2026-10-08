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

## Requirements

macOS, git and the GitHub CLI (`gh`). Optional: tmux, Neovim, terminal-notifier and Claude Code for AI reviews and brag summaries.

## Releases

Every push to `main` passes vet and tests, then GitHub Actions bumps the patch version in `.version`. That commit tags the release, publishes the binaries and updates the Homebrew formula. For a minor or major release, change `.version` yourself in the commit.

## Licence

[MIT](LICENSE)
