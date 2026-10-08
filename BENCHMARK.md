# Benchmarks

How fast digest is, measured in CI on every release, beside the release build. Each cell is time per operation · memory allocated per operation, so lower is better. A 60 fps frame is 16.7 ms.

- The release workflow records each run here in its own `chore: benchmark` commit.
- `mise run bench` prints the same tables locally without recording them; `mise run bench:full` adds 100k notes.
- History rows from different machines aren't directly comparable.

What runs, at 1, 100, 1k and 10k notes on disk:

- **Process**: starting the binary (`digest --help`) and its peak memory.
- **Startup and loading**: opening the dashboard (config, notes from disk, first frame), the heap it keeps, listing notes, reading local review state.
- **Redraw and navigation**: moving with j/k, the sync pulse, the header, opening a preview and stepping through previews.
- **Actions**: quick actions, a search keystroke, saving and deleting a note (each reloads every note).
- **Screens and overlays**: archive, help, settings, brag view, review details, link menu, delete confirm, error.
- **Typing in a long note**: a keystroke in a 500, 2k and 10k-line note with the cursor at the top and at the bottom.

## Latest

v1.2.5 working tree · 2026-10-08 15:51 UTC · Apple M1 Pro

### Process

| Process start (`digest --help`) | Peak memory |
|---|---|
| 18.4 ms | 18.1 MB |

### Startup and loading

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Startup | 1.21 ms · 676 KB | 9.9 ms · 2.0 MB | 94.9 ms · 13.3 MB | 978 ms · 127.2 MB |
| Startup heap | 0.5 MB | 0.6 MB | 1.6 MB | 9.4 MB |
| Load notes | 421 µs · 17 KB | 10.3 ms · 1.2 MB | 89.4 ms · 11.5 MB | 998 ms · 114.9 MB |
| Load reviews | 232 µs · 6 KB | 28.1 ms · 527 KB | 232 ms · 5.3 MB | 254 ms · 5.3 MB |

### Redraw and navigation

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Navigate j/k | 636 µs · 246 KB | 922 µs · 286 KB | 1.18 ms · 515 KB | 21.8 ms · 8.0 MB |
| Pulse tick | 169 µs · 84 KB | 181 µs · 84 KB | 224 µs · 95 KB | 718 µs · 228 KB |
| Header | 32.9 µs · 14 KB | 39.1 µs · 14 KB | 74.4 µs · 15 KB | 407 µs · 33 KB |
| Preview open | 630 µs · 372 KB | 626 µs · 388 KB | 1.04 ms · 708 KB | 12.4 ms · 6.3 MB |
| Preview next | 2.7 ms · 2.0 MB | 1.17 ms · 924 KB | 1.59 ms · 1.3 MB | 6.21 ms · 6.1 MB |

### Actions

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Quick actions open | 615 µs · 231 KB | 705 µs · 259 KB | 1.06 ms · 437 KB | 20.9 ms · 7.1 MB |
| Quick actions move | 607 µs · 208 KB | 660 µs · 225 KB | 961 µs · 330 KB | 19.6 ms · 6.2 MB |
| Search keystroke | 427 µs · 320 KB | 662 µs · 373 KB | 855 µs · 448 KB | 2.85 ms · 1.3 MB |
| Save note | 1.42 ms · 266 KB | 10.1 ms · 1.4 MB | 86.7 ms · 12.2 MB | 979 ms · 125.7 MB |
| Delete note | 1.28 ms · 237 KB | 10.2 ms · 1.4 MB | 91.9 ms · 12.2 MB | 1.03 s · 125.6 MB |

### Screens and overlays

Columns are notes on disk (reviews for Load reviews, at most 1000).

| Scenario | 1 | 100 | 1000 | 10000 |
|---|---|---|---|---|
| Archive | 109 µs · 86 KB | 471 µs · 280 KB | 1.34 ms · 578 KB | 7.83 ms · 2.3 MB |
| Help | 74 µs · 40 KB | 78.2 µs · 48 KB | 122 µs · 95 KB | 504 µs · 606 KB |
| Settings | 60.9 µs · 20 KB | 58.8 µs · 20 KB | 58.1 µs · 20 KB | 56.1 µs · 19 KB |
| Brag view | 1.84 ms · 1.5 MB | 1.86 ms · 1.5 MB | 1.89 ms · 1.5 MB | 1.77 ms · 1.5 MB |
| Review details | 736 µs · 577 KB | 752 µs · 612 KB | 897 µs · 799 KB | 2.47 ms · 2.8 MB |
| Link menu | 640 µs · 211 KB | 667 µs · 227 KB | 972 µs · 332 KB | 19.3 ms · 6.2 MB |
| Delete confirm | 9 µs · 3 KB | 8.85 µs · 3 KB | 9.3 µs · 3 KB | 8.63 µs · 3 KB |
| Error | 168 µs · 74 KB | 177 µs · 75 KB | 222 µs · 85 KB | 728 µs · 222 KB |

### Typing in a long note

One keystroke plus redraw.

| Note length | Cursor at top | Cursor at bottom |
|---|---|---|
| 500 lines | 1.42 ms · 759 KB | 4.28 ms · 2.0 MB |
| 2000 lines | 4.74 ms · 2.6 MB | 16.6 ms · 7.6 MB |
| 10000 lines | 738 ms · 81.2 MB | 81.3 ms · 37.2 MB |

## History

Newest first. Startup, Navigate and Save at 10k notes; Typing in a 10k-line note with the cursor at the bottom.

<!-- history:start -->
| Date | Version | Commit | Machine | Process start | Peak memory | Startup | Typing | Navigate | Save |
|---|---|---|---|---|---|---|---|---|---|
| 2026-10-08 | v1.2.5 | working tree | Apple M1 Pro | 18.4 ms | 18.1 MB | 978 ms | 81.3 ms | 21.8 ms | 979 ms |
<!-- history:end -->
