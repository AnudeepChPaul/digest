# Benchmarks

How fast digest is, measured in CI on every release, beside the release build. Each cell is time per operation · memory allocated per operation, so lower is better. A 60 fps frame is 16.7 ms.

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

v1.3.0 `30db267` · 2026-10-08 18:04 UTC · Apple M1 (Virtual)

### Process

| Process start (`digest --help`) | Peak memory |
|---|---|
| 30.4 ms | 17.5 MB |

### Startup and loading

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Startup | 779 µs · 684 KB | 5.3 ms · 1.9 MB | 37 ms · 9.4 MB | 273 ms · 82.7 MB |
| Startup heap | 0.6 MB | 0.7 MB | 1.3 MB | 6.4 MB |
| Load notes | 85.1 µs · 17 KB | 3.54 ms · 1.2 MB | 34.9 ms · 12.1 MB | 815 ms · 119.8 MB |
| Load reviews | 40.7 µs · 6 KB | 4.07 ms · 526 KB | 40 ms · 5.3 MB | 47.1 ms · 5.3 MB |

### Redraw and navigation

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Navigate j/k | 1.01 ms · 246 KB | 1.14 ms · 287 KB | 1.7 ms · 518 KB | 42.9 ms · 8.1 MB |
| Pulse tick | 439 µs · 84 KB | 363 µs · 84 KB | 372 µs · 95 KB | 1.29 ms · 230 KB |
| Header | 61.3 µs · 14 KB | 69.2 µs · 14 KB | 99.7 µs · 14 KB | 403 µs · 14 KB |
| Preview open | 1.31 ms · 363 KB | 1.08 ms · 385 KB | 1.49 ms · 705 KB | 15.5 ms · 6.4 MB |
| Preview next | 3.67 ms · 2.0 MB | 2.25 ms · 912 KB | 2.26 ms · 1.3 MB | 6.43 ms · 6.2 MB |

### Actions

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Quick actions open | 822 µs · 230 KB | 1.01 ms · 259 KB | 1.41 ms · 436 KB | 30.9 ms · 7.4 MB |
| Quick actions move | 758 µs · 208 KB | 865 µs · 226 KB | 1.32 ms · 329 KB | 22.3 ms · 6.3 MB |
| Search keystroke | 724 µs · 316 KB | 899 µs · 369 KB | 1.2 ms · 423 KB | 3.27 ms · 968 KB |
| Save note | 1.79 ms · 268 KB | 4.58 ms · 1.3 MB | 29.8 ms · 8.1 MB | 367 ms · 81.5 MB |
| Delete note | 1.4 ms · 236 KB | 4.26 ms · 1.3 MB | 22.4 ms · 8.0 MB | 307 ms · 81.3 MB |

### Screens and overlays

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Archive | 145 µs · 84 KB | 140 µs · 84 KB | 140 µs · 84 KB | 168 µs · 83 KB |
| Help | 96.1 µs · 40 KB | 101 µs · 47 KB | 152 µs · 94 KB | 752 µs · 608 KB |
| Settings | 83.3 µs · 19 KB | 71.8 µs · 19 KB | 102 µs · 19 KB | 90.6 µs · 19 KB |
| Brag view | 2.58 ms · 1.5 MB | 2.79 ms · 1.5 MB | 2.44 ms · 1.5 MB | 2.6 ms · 1.5 MB |
| Review details | 1.47 ms · 575 KB | 1.13 ms · 612 KB | 1.59 ms · 795 KB | 3.81 ms · 2.8 MB |
| Link menu | 1.14 ms · 211 KB | 1.41 ms · 227 KB | 2.05 ms · 333 KB | 30.4 ms · 6.4 MB |
| Delete confirm | 18.9 µs · 3 KB | 13.1 µs · 3 KB | 12.6 µs · 3 KB | 10.9 µs · 3 KB |
| Error | 205 µs · 74 KB | 297 µs · 74 KB | 322 µs · 84 KB | 802 µs · 206 KB |

### Typing in a long note

One keystroke plus redraw.

| Note length | Cursor at top | Cursor at bottom |
|---|---|---|
| 500 lines | 2.26 ms · 755 KB | 6.56 ms · 2.0 MB |
| 2000 lines | 7.09 ms · 2.6 MB | 28 ms · 7.6 MB |
| 10000 lines | 1.34 s · 81.2 MB | 121 ms · 37.2 MB |

## History

One column per run, newest first; re-recording a version replaces its column. Rows are every scenario at 1k and at 10k notes; the 10k table adds process start, peak memory and typing in a long note.

<!-- history:start -->
### 1k notes

| Scenario | v1.3.0<br>2026-10-08<br>`30db267`<br>Apple M1 (Virtual) |
|---|---|
| Startup | 37 ms · 9.4 MB |
| Startup heap | 1.3 MB |
| Load notes | 34.9 ms · 12.1 MB |
| Load reviews | 40 ms · 5.3 MB |
| Navigate j/k | 1.7 ms · 518 KB |
| Pulse tick | 372 µs · 95 KB |
| Header | 99.7 µs · 14 KB |
| Preview open | 1.49 ms · 705 KB |
| Preview next | 2.26 ms · 1.3 MB |
| Quick actions open | 1.41 ms · 436 KB |
| Quick actions move | 1.32 ms · 329 KB |
| Search keystroke | 1.2 ms · 423 KB |
| Save note | 29.8 ms · 8.1 MB |
| Delete note | 22.4 ms · 8.0 MB |
| Archive | 140 µs · 84 KB |
| Help | 152 µs · 94 KB |
| Settings | 102 µs · 19 KB |
| Brag view | 2.44 ms · 1.5 MB |
| Review details | 1.59 ms · 795 KB |
| Link menu | 2.05 ms · 333 KB |
| Delete confirm | 12.6 µs · 3 KB |
| Error | 322 µs · 84 KB |

### 10k notes

| Scenario | v1.3.0<br>2026-10-08<br>`30db267`<br>Apple M1 (Virtual) | v1.2.5<br>2026-10-08<br>working tree<br>Apple M1 Pro |
|---|---|---|
| Process start | 30.4 ms | 18.4 ms |
| Peak memory | 17.5 MB | 18.1 MB |
| Startup | 273 ms · 82.7 MB | 978 ms |
| Startup heap | 6.4 MB | – |
| Load notes | 815 ms · 119.8 MB | – |
| Load reviews | 47.1 ms · 5.3 MB | – |
| Navigate j/k | 42.9 ms · 8.1 MB | 21.8 ms |
| Pulse tick | 1.29 ms · 230 KB | – |
| Header | 403 µs · 14 KB | – |
| Preview open | 15.5 ms · 6.4 MB | – |
| Preview next | 6.43 ms · 6.2 MB | – |
| Quick actions open | 30.9 ms · 7.4 MB | – |
| Quick actions move | 22.3 ms · 6.3 MB | – |
| Search keystroke | 3.27 ms · 968 KB | – |
| Save note | 367 ms · 81.5 MB | 979 ms |
| Delete note | 307 ms · 81.3 MB | – |
| Archive | 168 µs · 83 KB | – |
| Help | 752 µs · 608 KB | – |
| Settings | 90.6 µs · 19 KB | – |
| Brag view | 2.6 ms · 1.5 MB | – |
| Review details | 3.81 ms · 2.8 MB | – |
| Link menu | 30.4 ms · 6.4 MB | – |
| Delete confirm | 10.9 µs · 3 KB | – |
| Error | 802 µs · 206 KB | – |
| Typing 500 lines, cursor top | 2.26 ms · 755 KB | – |
| Typing 500 lines, cursor bottom | 6.56 ms · 2.0 MB | – |
| Typing 2000 lines, cursor top | 7.09 ms · 2.6 MB | – |
| Typing 2000 lines, cursor bottom | 28 ms · 7.6 MB | – |
| Typing 10000 lines, cursor top | 1.34 s · 81.2 MB | – |
| Typing 10000 lines, cursor bottom | 121 ms · 37.2 MB | 81.3 ms |

<!-- history:end -->
