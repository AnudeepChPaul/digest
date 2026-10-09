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

v1.4.0 `46988ad` · 2026-10-09 09:55 UTC · Apple M1 (Virtual)

### Process

| Process start (`digest --help`) | Peak memory |
|---|---|
| 18.9 ms | 18.0 MB |

### Startup and loading

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Startup | 932 µs · 723 KB | 6.12 ms · 1.9 MB | 42.3 ms · 9.2 MB | 352 ms · 82.5 MB |
| Startup heap | 0.6 MB | 0.7 MB | 1.3 MB | 6.4 MB |
| Load notes | 135 µs · 17 KB | 6.44 ms · 1.2 MB | 51 ms · 12.0 MB | 830 ms · 119.7 MB |
| Load reviews | 55.5 µs · 6 KB | 5.74 ms · 526 KB | 68.9 ms · 5.3 MB | 62.6 ms · 5.3 MB |

### Redraw and navigation

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Navigate j/k | 1.65 ms · 271 KB | 1.46 ms · 312 KB | 2.06 ms · 537 KB | 36.1 ms · 8.2 MB |
| Pulse tick | 358 µs · 84 KB | 294 µs · 83 KB | 320 µs · 94 KB | 874 µs · 214 KB |
| Header | 47.4 µs · 14 KB | 63.5 µs · 14 KB | 117 µs · 14 KB | 366 µs · 14 KB |
| Preview open | 1.07 ms · 391 KB | 1.46 ms · 406 KB | 2.04 ms · 727 KB | 18.2 ms · 6.4 MB |
| Preview next | 3.51 ms · 1.5 MB | 2.2 ms · 938 KB | 3.03 ms · 1.4 MB | 8.74 ms · 6.3 MB |

### Actions

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Quick actions open | 2.2 ms · 254 KB | 1.25 ms · 283 KB | 2.2 ms · 461 KB | 34.5 ms · 7.4 MB |
| Quick actions move | 929 µs · 232 KB | 1.2 ms · 248 KB | 1.48 ms · 357 KB | 31 ms · 6.4 MB |
| Search keystroke | 724 µs · 336 KB | 1.02 ms · 386 KB | 1.36 ms · 442 KB | 3.67 ms · 991 KB |
| Save note | 1.57 ms · 231 KB | 2.33 ms · 264 KB | 3.51 ms · 485 KB | 34.8 ms · 6.9 MB |
| Delete note | 1.24 ms · 195 KB | 1.03 ms · 218 KB | 1.54 ms · 373 KB | 28.8 ms · 6.7 MB |

### Screens and overlays

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Archive | 178 µs · 113 KB | 183 µs · 113 KB | 181 µs · 113 KB | 166 µs · 112 KB |
| Help | 320 µs · 100 KB | 322 µs · 108 KB | 341 µs · 154 KB | 1.01 ms · 671 KB |
| Settings | 105 µs · 19 KB | 127 µs · 19 KB | 119 µs · 19 KB | 96.7 µs · 19 KB |
| Brag view | 2.79 ms · 1.5 MB | 2.89 ms · 1.5 MB | 2.56 ms · 1.5 MB | 2.93 ms · 1.5 MB |
| Review details | 1.33 ms · 576 KB | 1.54 ms · 609 KB | 1.27 ms · 797 KB | 3 ms · 2.8 MB |
| Link menu | 1.05 ms · 234 KB | 976 µs · 250 KB | 1.88 ms · 358 KB | 35.6 ms · 6.5 MB |
| Delete confirm | 12.4 µs · 3 KB | 12.7 µs · 3 KB | 14.1 µs · 3 KB | 14.7 µs · 3 KB |
| Error | 252 µs · 74 KB | 282 µs · 73 KB | 308 µs · 84 KB | 1.04 ms · 209 KB |

### Typing in a long note

One keystroke plus redraw.

| Note length | Cursor at top | Cursor at bottom |
|---|---|---|
| 500 lines | 2.3 ms · 771 KB | 6.72 ms · 2.0 MB |
| 2000 lines | 8.33 ms · 2.6 MB | 26.4 ms · 7.6 MB |
| 10000 lines | 1.31 s · 81.2 MB | 144 ms · 37.2 MB |

## History

One column per run, newest first; re-recording a version replaces its column. Rows are every scenario at 1k and at 10k notes; the 10k table adds process start, peak memory and typing in a long note.

<!-- history:start -->
### 1k notes

| Scenario | v1.4.0<br>2026-10-09<br>`46988ad`<br>Apple M1 (Virtual) | v1.3.0<br>2026-10-08<br>`30db267`<br>Apple M1 (Virtual) |
|---|---|---|
| Startup | 42.3 ms · 9.2 MB | 37 ms · 9.4 MB |
| Startup heap | 1.3 MB | 1.3 MB |
| Load notes | 51 ms · 12.0 MB | 34.9 ms · 12.1 MB |
| Load reviews | 68.9 ms · 5.3 MB | 40 ms · 5.3 MB |
| Navigate j/k | 2.06 ms · 537 KB | 1.7 ms · 518 KB |
| Pulse tick | 320 µs · 94 KB | 372 µs · 95 KB |
| Header | 117 µs · 14 KB | 99.7 µs · 14 KB |
| Preview open | 2.04 ms · 727 KB | 1.49 ms · 705 KB |
| Preview next | 3.03 ms · 1.4 MB | 2.26 ms · 1.3 MB |
| Quick actions open | 2.2 ms · 461 KB | 1.41 ms · 436 KB |
| Quick actions move | 1.48 ms · 357 KB | 1.32 ms · 329 KB |
| Search keystroke | 1.36 ms · 442 KB | 1.2 ms · 423 KB |
| Save note | 3.51 ms · 485 KB | 29.8 ms · 8.1 MB |
| Delete note | 1.54 ms · 373 KB | 22.4 ms · 8.0 MB |
| Archive | 181 µs · 113 KB | 140 µs · 84 KB |
| Help | 341 µs · 154 KB | 152 µs · 94 KB |
| Settings | 119 µs · 19 KB | 102 µs · 19 KB |
| Brag view | 2.56 ms · 1.5 MB | 2.44 ms · 1.5 MB |
| Review details | 1.27 ms · 797 KB | 1.59 ms · 795 KB |
| Link menu | 1.88 ms · 358 KB | 2.05 ms · 333 KB |
| Delete confirm | 14.1 µs · 3 KB | 12.6 µs · 3 KB |
| Error | 308 µs · 84 KB | 322 µs · 84 KB |

### 10k notes

| Scenario | v1.4.0<br>2026-10-09<br>`46988ad`<br>Apple M1 (Virtual) | v1.3.0<br>2026-10-08<br>`30db267`<br>Apple M1 (Virtual) | v1.2.5<br>2026-10-08<br>working tree<br>Apple M1 Pro |
|---|---|---|---|
| Process start | 18.9 ms | 30.4 ms | 18.4 ms |
| Peak memory | 18.0 MB | 17.5 MB | 18.1 MB |
| Startup | 352 ms · 82.5 MB | 273 ms · 82.7 MB | 978 ms |
| Startup heap | 6.4 MB | 6.4 MB | – |
| Load notes | 830 ms · 119.7 MB | 815 ms · 119.8 MB | – |
| Load reviews | 62.6 ms · 5.3 MB | 47.1 ms · 5.3 MB | – |
| Navigate j/k | 36.1 ms · 8.2 MB | 42.9 ms · 8.1 MB | 21.8 ms |
| Pulse tick | 874 µs · 214 KB | 1.29 ms · 230 KB | – |
| Header | 366 µs · 14 KB | 403 µs · 14 KB | – |
| Preview open | 18.2 ms · 6.4 MB | 15.5 ms · 6.4 MB | – |
| Preview next | 8.74 ms · 6.3 MB | 6.43 ms · 6.2 MB | – |
| Quick actions open | 34.5 ms · 7.4 MB | 30.9 ms · 7.4 MB | – |
| Quick actions move | 31 ms · 6.4 MB | 22.3 ms · 6.3 MB | – |
| Search keystroke | 3.67 ms · 991 KB | 3.27 ms · 968 KB | – |
| Save note | 34.8 ms · 6.9 MB | 367 ms · 81.5 MB | 979 ms |
| Delete note | 28.8 ms · 6.7 MB | 307 ms · 81.3 MB | – |
| Archive | 166 µs · 112 KB | 168 µs · 83 KB | – |
| Help | 1.01 ms · 671 KB | 752 µs · 608 KB | – |
| Settings | 96.7 µs · 19 KB | 90.6 µs · 19 KB | – |
| Brag view | 2.93 ms · 1.5 MB | 2.6 ms · 1.5 MB | – |
| Review details | 3 ms · 2.8 MB | 3.81 ms · 2.8 MB | – |
| Link menu | 35.6 ms · 6.5 MB | 30.4 ms · 6.4 MB | – |
| Delete confirm | 14.7 µs · 3 KB | 10.9 µs · 3 KB | – |
| Error | 1.04 ms · 209 KB | 802 µs · 206 KB | – |
| Typing 500 lines, cursor top | 2.3 ms · 771 KB | 2.26 ms · 755 KB | – |
| Typing 500 lines, cursor bottom | 6.72 ms · 2.0 MB | 6.56 ms · 2.0 MB | – |
| Typing 2000 lines, cursor top | 8.33 ms · 2.6 MB | 7.09 ms · 2.6 MB | – |
| Typing 2000 lines, cursor bottom | 26.4 ms · 7.6 MB | 28 ms · 7.6 MB | – |
| Typing 10000 lines, cursor top | 1.31 s · 81.2 MB | 1.34 s · 81.2 MB | – |
| Typing 10000 lines, cursor bottom | 144 ms · 37.2 MB | 121 ms · 37.2 MB | 81.3 ms |

<!-- history:end -->
