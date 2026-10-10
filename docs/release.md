# Releases

digest follows [Semantic Versioning](https://semver.org), and the commit messages since the last release decide the next version, following [Conventional Commits](https://www.conventionalcommits.org):

| Commits since the last release | Release |
|---|---|
| `feat!:`, `fix(scope)!:` or a `BREAKING CHANGE:` line in the body | major |
| `feat:` | minor |
| `fix:` or `perf:` | patch |
| only `docs:`, `ci:`, `chore:`, `test:` and the like | none |

Versions start at v0.0.0 under the module path `github.com/achandrapaul/digest`. The Go module proxy still lists v1.1.50 to v1.3.0 under the old `github.com/AnudeepChPaul/digest` path; those are stale and no longer updated.

The last release is the commit of the newest published GitHub release tag, found with `gh release view`; without one it is the newest commit that changed `.version`. `mise run release:level` and `mise run release:notes` look at every commit after it.

## Cutting a release

Pushes to `main` never release on their own. To cut a version:

```sh
mise run release:level          # prints the level the commits call for
mise run release:commit         # patch by default; pass minor or major to choose
```

`release:commit` bumps `.version`, adds the new [CHANGELOG.md](../CHANGELOG.md) section with `mise run release:notes`, commits both as `release:<patch|minor|major>` and pushes the current branch. On `main` it only pushes; on any other branch it also opens a pull request into `main`. The section lists every commit since the last published release, including those of versions tagged but never published, under Changes, as a link to the commit, its title and its description; release, changelog and benchmark commits are left out.

A push to `main` that changes `.version` runs the build workflow. It runs vet and tests and tags `v<version>`. Nothing is published yet.

## Publishing a release

Run the release workflow from the Actions tab, or `gh workflow run release.yml -f tag=v<version>`; without a tag it takes the latest one. It checks out the tag, publishes the macOS binaries on GitHub with that version's changelog section as release notes, asks the Go module proxy for the tag, and points the Homebrew formula at it. Older tags and releases stay; GitHub marks the newest one as Latest.

## Benchmarks

The release workflow records how fast digest starts, loads notes, redraws, searches and saves, at 1 to 10k notes, in [benchmark.md](benchmark/benchmark.md) as its own `chore: benchmark` commit.

`mise run bench` measures locally without recording, and `mise run bench:full` adds 100k notes.
