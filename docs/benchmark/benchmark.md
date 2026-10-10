# Benchmarks

How fast digest is, measured in CI on every version bump you push, beside the release build. Each cell is time per operation · memory allocated per operation, so lower is better. A 60 fps frame is 16.7 ms.

- The release workflow records each run here in its own `chore: benchmark` commit.
- `mise run bench` prints the same tables locally without recording them; `mise run bench:full` adds 100k notes.
- History columns from different machines aren't directly comparable.

What runs, at 1, 100, 1k and 10k notes on disk:

- **Process**: starting the binary (`digest --help`) and its peak memory.
- **Startup and loading**: opening the dashboard (config, notes from disk, first frame), the heap it keeps, listing notes, reading local review state.
- **Redraw and navigation**: moving with j/k, the sync pulse, the header, opening a preview and stepping through previews.
- **Actions**: quick actions, a search keystroke, saving and deleting a note (each reloads only that note).
- **Screens and overlays**: archive, help, settings, brag view, review details, link menu, delete confirm, error.
- **Typing in a long note**: a keystroke in a 500, 2k and 10k-line note with the cursor at the top and at the bottom.

## Latest

v0.0.152 `c562e46` · 2026-10-10 22:38 UTC · Apple M1 (Virtual)

### Process

| Process start (`digest --help`) | Peak memory |
|---|---|
| 3.99 ms | 7.3 MB |

### Startup and loading

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Startup | 523 µs · 744 KB | 3.47 ms · 1.9 MB | 22.7 ms · 9.5 MB | 215 ms · 82.8 MB |
| Startup heap | 0.6 MB | 0.7 MB | 1.3 MB | 6.4 MB |
| Load notes | 78.2 µs · 17 KB | 3 ms · 1.2 MB | 32.3 ms · 12.0 MB | 362 ms · 119.8 MB |
| Load reviews | 33.9 µs · 6 KB | 3.45 ms · 526 KB | 36.6 ms · 5.3 MB | 36.8 ms · 5.3 MB |

### Redraw and navigation

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Navigate j/k | 899 µs · 296 KB | 833 µs · 345 KB | 1.35 ms · 654 KB | 23.5 ms · 8.6 MB |
| Pulse tick | 183 µs · 89 KB | 196 µs · 89 KB | 237 µs · 99 KB | 572 µs · 208 KB |
| Header | 31.6 µs · 12 KB | 40 µs · 12 KB | 64.9 µs · 13 KB | 260 µs · 13 KB |
| Preview open | 474 µs · 280 KB | 537 µs · 338 KB | 999 µs · 744 KB | 16.6 ms · 6.6 MB |
| Preview next | 593 µs · 299 KB | 1.09 ms · 622 KB | 1.4 ms · 1.2 MB | 3.79 ms · 6.5 MB |

### Actions

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Quick actions open | 879 µs · 273 KB | 821 µs · 304 KB | 1.54 ms · 527 KB | 28.3 ms · 7.5 MB |
| Quick actions move | 711 µs · 246 KB | 766 µs · 261 KB | 1.14 ms · 369 KB | 32.4 ms · 6.3 MB |
| Search keystroke | 508 µs · 351 KB | 731 µs · 404 KB | 929 µs · 468 KB | 2.76 ms · 1017 KB |
| Save note | 944 µs · 240 KB | 946 µs · 271 KB | 1.93 ms · 494 KB | 23.2 ms · 6.7 MB |
| Delete note | 778 µs · 204 KB | 805 µs · 226 KB | 1.19 ms · 380 KB | 24.9 ms · 6.7 MB |

### Screens and overlays

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Archive | 137 µs · 130 KB | 118 µs · 130 KB | 121 µs · 131 KB | 157 µs · 129 KB |
| Help | 210 µs · 119 KB | 244 µs · 134 KB | 307 µs · 250 KB | 743 µs · 1.2 MB |
| Settings | 63.6 µs · 24 KB | 63.4 µs · 24 KB | 62.7 µs · 24 KB | 61.7 µs · 24 KB |
| Brag view | 137 µs · 78 KB | 113 µs · 79 KB | 116 µs · 79 KB | 108 µs · 77 KB |
| Review details | 154 µs · 139 KB | 173 µs · 172 KB | 296 µs · 398 KB | 1.22 ms · 2.4 MB |
| Link menu | 704 µs · 248 KB | 736 µs · 264 KB | 967 µs · 371 KB | 19.3 ms · 6.2 MB |
| Delete confirm | 14.6 µs · 8 KB | 14.6 µs · 8 KB | 14.6 µs · 8 KB | 14.1 µs · 8 KB |
| Error | 154 µs · 77 KB | 165 µs · 77 KB | 203 µs · 86 KB | 497 µs · 191 KB |

### Typing in a long note

One keystroke plus redraw.

| Note length | Cursor at top | Cursor at bottom |
|---|---|---|
| 500 lines | 1.62 ms · 799 KB | 4.85 ms · 2.0 MB |
| 2000 lines | 5.23 ms · 2.6 MB | 18.1 ms · 7.6 MB |
| 10000 lines | 984 ms · 81.2 MB | 101 ms · 37.2 MB |

## History

One column per run, newest first; re-recording a version replaces its column. Rows are every scenario at 1k and at 10k notes; the 10k table adds process start, peak memory and typing in a long note.

<!-- history:start -->
### 1k notes

| Scenario | v0.0.152<br>2026-10-10<br>`c562e46`<br>Apple M1 (Virtual) |
|---|---|
| Startup | 22.7 ms · 9.5 MB |
| Startup heap | 1.3 MB |
| Load notes | 32.3 ms · 12.0 MB |
| Load reviews | 36.6 ms · 5.3 MB |
| Navigate j/k | 1.35 ms · 654 KB |
| Pulse tick | 237 µs · 99 KB |
| Header | 64.9 µs · 13 KB |
| Preview open | 999 µs · 744 KB |
| Preview next | 1.4 ms · 1.2 MB |
| Quick actions open | 1.54 ms · 527 KB |
| Quick actions move | 1.14 ms · 369 KB |
| Search keystroke | 929 µs · 468 KB |
| Save note | 1.93 ms · 494 KB |
| Delete note | 1.19 ms · 380 KB |
| Archive | 121 µs · 131 KB |
| Help | 307 µs · 250 KB |
| Settings | 62.7 µs · 24 KB |
| Brag view | 116 µs · 79 KB |
| Review details | 296 µs · 398 KB |
| Link menu | 967 µs · 371 KB |
| Delete confirm | 14.6 µs · 8 KB |
| Error | 203 µs · 86 KB |

### 10k notes

| Scenario | v0.0.152<br>2026-10-10<br>`c562e46`<br>Apple M1 (Virtual) |
|---|---|
| Process start | 3.99 ms |
| Peak memory | 7.3 MB |
| Startup | 215 ms · 82.8 MB |
| Startup heap | 6.4 MB |
| Load notes | 362 ms · 119.8 MB |
| Load reviews | 36.8 ms · 5.3 MB |
| Navigate j/k | 23.5 ms · 8.6 MB |
| Pulse tick | 572 µs · 208 KB |
| Header | 260 µs · 13 KB |
| Preview open | 16.6 ms · 6.6 MB |
| Preview next | 3.79 ms · 6.5 MB |
| Quick actions open | 28.3 ms · 7.5 MB |
| Quick actions move | 32.4 ms · 6.3 MB |
| Search keystroke | 2.76 ms · 1017 KB |
| Save note | 23.2 ms · 6.7 MB |
| Delete note | 24.9 ms · 6.7 MB |
| Archive | 157 µs · 129 KB |
| Help | 743 µs · 1.2 MB |
| Settings | 61.7 µs · 24 KB |
| Brag view | 108 µs · 77 KB |
| Review details | 1.22 ms · 2.4 MB |
| Link menu | 19.3 ms · 6.2 MB |
| Delete confirm | 14.1 µs · 8 KB |
| Error | 497 µs · 191 KB |
| Typing 500 lines, cursor top | 1.62 ms · 799 KB |
| Typing 500 lines, cursor bottom | 4.85 ms · 2.0 MB |
| Typing 2000 lines, cursor top | 5.23 ms · 2.6 MB |
| Typing 2000 lines, cursor bottom | 18.1 ms · 7.6 MB |
| Typing 10000 lines, cursor top | 984 ms · 81.2 MB |
| Typing 10000 lines, cursor bottom | 101 ms · 37.2 MB |

<!-- history:end -->
