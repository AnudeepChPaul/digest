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

Then run `digest install` to add any missing tools, the reminder launchd agent and a keyboard shortcut. Each step asks first.

## Usage

```sh
digest            # open the dashboard; the first run walks through setup
digest doctor     # check the tools digest needs
digest setup      # run setup again
digest --help     # every command and flag
```

Inside the dashboard, `,` opens settings and the footer lists the keys for the current screen.

## Requirements

macOS, git and the GitHub CLI (`gh`). Optional: tmux, Neovim, terminal-notifier and Claude Code for AI reviews and brag summaries.

## Releases

Every push to `main` passes vet and tests, then GitHub Actions bumps the patch version in `.version`. That commit tags the release, publishes the binaries and updates the Homebrew formula. For a minor or major release, change `.version` yourself in the commit.

## Licence

[MIT](LICENSE)
