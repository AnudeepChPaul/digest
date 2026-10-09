# Releases

digest follows [Semantic Versioning](https://semver.org), and the commit messages since the last release decide the next version, following [Conventional Commits](https://www.conventionalcommits.org):

| Commits since the last release | Release |
|---|---|
| `feat!:`, `fix(scope)!:` or a `BREAKING CHANGE:` line in the body | major |
| `feat:` | minor |
| `fix:` or `perf:` | patch |
| only `docs:`, `ci:`, `chore:`, `test:` and the like | none |

The last release is the newest commit that changed `.version`. `mise run release:level` and the changelog look at every commit after it.

## Cutting a release

Pushes to `main` never release on their own. To release:

```sh
mise run release:level      # prints the level the commits call for
mise run release:commit     # bumps .version to that level and commits it; pass patch, minor or major to choose
git push
```

A push that changes `.version` runs the release workflow. It runs vet and tests, writes the new [CHANGELOG.md](../CHANGELOG.md) section from the commits since the previous release, tags `v<version>`, publishes the macOS binaries with that section as release notes, updates the Homebrew formula, and commits `CHANGELOG.md` to `main`. Never edit `CHANGELOG.md` locally. Older tags and releases stay; GitHub marks the newest one as Latest.

## Benchmarks

The same workflow records how fast digest starts, loads notes, redraws, searches and saves, at 1 to 10k notes, in [benchmark.md](benchmark/benchmark.md) as its own `chore: benchmark` commit.

`mise run bench` measures locally without recording, and `mise run bench:full` adds 100k notes.
