Everywhere

- ctrl+c — works in any view: each press counts toward quitting, the 3rd within 2s stops git sync and exits, and the count resets 2s after each press.
- ctrl+c warning — the dashboard footer shows "ctrl+c N more"; other popups show "ctrl+c N more to quit" on their bottom line.
- Narrow terminal — under 40 columns the screen only says "Terminal window is too small."
- Scroll keys in preview, archive, brag view and search preview — j/k/arrows, pgup/pgdown, ctrl+d/ctrl+u.
- ctrl+d/ctrl+u in text boxes — move half a page; forward-delete is the delete key only.
- Errors never open a popup —r as "✗ source: text" and stay in the message log.

Dashboard: what you see

- Header — the "digest" banner plays a wave animation once at startup.
- Header, top line — "🔥 N-da row with notes closed) and "N closed this week".
- Header, top right — the newng", "✓ git synced" (fadesafter 4s), or "✗ source: error" (stays until esc).
- Header, middle line — versid a yellow "Last week (Wnn)isn't bragged — press b" when last week has no brag.
- Yesterday — notes closed ontitled "Y E S T E R D A Y ·date" or the spaced weekday name.
- Today — "T O D A Y · date" nother day) with a "N jobs ◆"badge on the right.
- Today: Pending, Carried Oveted before the day you'reviewing.
- Today: Added Today (n) — ac day you're viewing.
- Today: Closed Today (n) — notes marked done on the day you're viewing.
- G I T strip — two columns, ith one row per repo: "Nassigned · N reviewed · N commits".
- M Y P R ( S ) — your openum branch", "(draft)", files,age, an approved glyph, and CI (✓ ✗ ◌ ·).
- Pending Git Actions — PRs wrouped by repo, with the sorthint "s Updated|Created w ↓|↑ m scope".
- PR row tags — size ±N, age state (pending, reviewing,rec:X, approved, changes requested…), and first-review/re-review and direct/team
  icons.
- Jobs — configured jobs (◆, or ✔ after a clean dry run), review runs, brag runs and
  running automation runs.
- Note tags — "@notify:interval", "#draft" or "#jira" (automation), age (HH:MM,
  weekday, or yellow "Nd ago"
- When tags show — on the selected row only, unless show_tags: always; PR notes and
  the notify tag always show.
- Key hint pill — with show_key_hints on, a pill under the selected row lists its
  keys after 250ms of no typi
- j/k order — Yesterday, Carried, Added, Closed, git repos, My PRs, Pending PRs,
  jobs, review runs, brag run
- Startup selection — selection_default: notes_today or notes_yesterday picks where
  the cursor starts; otherwisrow.
- Startup git data — loads from the cache; a full sync runs only if the cache is
  stale.
- Auto-sync — runs a git sync every git_auto_sync_interval seconds (default 600).
- Day sync — p, n and t startlast press.
- PR alerts — after a sync, macOS notifications fire for approved, changes
  requested, merged, closed, sted and re-review; the firstsync is silent.
- Automatic PR review notes —te or update "#pr-review"notes; approved ones are closed, and they reopen if review is requested again.
- Automatic My PR notes — eacPR:" note with an "## Updates" log, closed with "Merged" or "Closed".
- Reminder cleanup — remindernote is done or archived.
- Git turned off — the strip and pending section are hidden, and g c h l ← → s w m
  stop working.

Dashboard: keys

- enter on a note — opens the; saving returns you to thepreview.
- enter on a pending PR or Mywser.
- enter on a job or run — opens its preview.
- enter on a git repo — does
- space — marks the note done or active and saves it.
- a — opens the editor for a u're viewing.
- i — edits the selected note's summary in place.
- @ or . — opens the action m
- o — opens the note's link, or a link menu when it has several (shown only if the
  note has URLs).
- d on a note — asks to archive it.
- tab — opens the preview; ondetails instead.
- t — jumps to today and reloads.
- p / n — moves to the previos.
- ctrl+e — opens the Archive.
- g — starts a git sync now,
- c — refreshes local commits only (the label shows only when daily commits are on).
- r on a job — asks to run it
- j/k/arrows — move the selection.
- ctrl+d/ctrl+u/pgdown/pgup —a page.
- h/l/←/→ — in the git strip, jump to the same row in the other day's column.
- / — opens Search.
- b — opens Brag.
- ? — opens Shortcuts.
- , (hidden) — opens the Settings form.
- ! (hidden) — opens the Mess
- s (hidden) — switches the pending-PR sort between Updated and Created; saved.
- w (hidden) — switches the pscending and ascending; saved.
- m (hidden) — shows only PRs that ask for you by name; not saved.
- esc (hidden) — hides headernc failures; does nothing when there are none.

Dashboard: keys on specific rows

- Pending PR y — asks to approve (only when show_key_hints is on).
- Pending PR d — stops a runnuests changes with theselected findings, or opens a comment box.
- Pending PR r — asks to star
- Pending PR o — opens the review clone in nvim (only once a clone exists).
- Job r — asks to run it.
- Job d — starts a background dry run when idle; asks to abort when running.
- Review run d — asks to stop
- Brag run d — asks to stop it when running; dismisses it straight away when failed.
- Automation run d — asks to failed).

Inline edit

- enter — saves the trimmed s
- esc — cancels.
- Other keys — type into the

Notify input (from @notify)

- Typing — accepts digits fol
- enter / y — saves the reminder: empty means 1h, the maximum is 3d, and no unit
  means hours.
- The saved reminder opens the note's PR URL, or your terminal.
- esc — cancels.
- Delivery — digest notify-due sends reminders via launchd every minute.

Action menu (@ / .)

- @notify — opens the reminder input.
- @notify:off — removes the r
- Automation items — jira ("create a ticket"), confluence ("confluence page"),
  google doc; listed when the
- Automation chosen — asks to draft it.
- Ordering — items are sortedm.
- j/k — move.
- enter / y — choose.
- esc / n — cancel.

Link menu

- enter / y — opens the link in the browser.
- j/k — move.
- esc / n — cancel.

Preview (all kinds)

- p / n — moves to the previous or next dashboard row inside the preview.
- ? — opens Shortcuts.
- esc (or tab where tabs don't apply) — closes.

Note preview

- enter — edits the note. space — marks it done or a
- @ / . — opens actions. o — opens links.
- d — asks to archive, then moves to the next row. ctrl+y — copies the summar
  te preview: Draft tab (whe)
  tab — switches between the
- enter — edits the draft. x — drafts again.
- r — asks to create the ticket or doc from the draft. d — deletes the draft.
- ctrl+y — copies the draft.  
  Pending PR preview
- Details tab — author, CI, wait time, size, Jira link, code owners, your review, files, and recent
- Re-reviews — the Details tab also shows commits and files changed since review.
- Review tab — Claude's recommendation, findings by severity with checkboxreview's progress or failu
- tab — switches tabs. y / a — asks to approve.
- d — stops the review, or requests changes with the selected findings, orcomment box.
- r — asks to start a review. o — opens the clone in a ne tmux only).
- space — selects the finding under the cursor. ctrl+a — selects or clears
- j/k — move between findings. enter — posts selected finwith none selected, opens thePR in the browser. ctrl+y — copies the title,
  PR preview
  What it shows — title, braision, reviewers anddescription. enter — opens the PR.
- ctrl+y — copies nothing.  
  Job preview
- What it shows — the live log, dry-run output, or the finished log (last lines); it refreshes every
- r / enter — asks to run. d — dry run, or stop when
- ctrl+y — copies the log.  
  Review / brag / automation run preview
- What it shows — the run's log tail, which updates as the run writes to id — stop or dismiss.
- ctrl+y — copies nothing.  
  Git details popup
- tab — cycles the filter: All, Reviewed, Assigned, Commits. enter — opens the item in
- j/k — move. esc — close.
  nfirm popups (run / abort
  y / enter on run — starts
- y / enter on abort — sends SIGTERM, then SIGKILL after 3s, and logs "[JOBY USER]".
- y / enter on stop — stops the brag or automation run. y / enter on archive — arc
- y / enter on delete (from the Archive) — deletes the note files permanenesc — cancel.
  view confirm
  y / enter — submits the apcomment review to GitHub,updates the review note, then resyncs. esc / n — cancel.
  art / stop review confirm
  y / enter — starts a Claudtall, review), which appearsin Jobs; or stops the running one. esc / n — cancel.
  ject comment box
  ctrl+s — continues to the efuses if the comment isempty. esc — cancel.
- ctrl+d/ctrl+u — move half a page.  
  Archive
- space — selects or deselects a note for a multi-note action. u / enter — restores the sent one) as active, dated theday you're viewing. d — asks to delete them pe
- j/k — move. esc / ctrl+e — close.
  te editor
  Layout — the first line isfter it is the body.
- ctrl+o — saves ("Untitled Note" if the first line is empty). ctrl+y — copies the text.
- esc — cancel. ctrl+d/ctrl+u — move half
  arch
  Plain words — must appear
- tag:x — matches the note's source or subject. date:7d / 2w / 3m / 1y — n
- date:DD-MM-YYYY — notes from that exact day. Scope — active and done noth matches highlighted.
- up/down — move. pgup/pgdn — page.
- ctrl+d/ctrl+u — half page. tab — opens the preview.
- ctrl+e — exports the results to ~/Downloads/search-<timestamp>.csv and cpath.
- esc — close.  
  Search preview
- n / p — next or previous result. enter — edit.
- ctrl+y — copy. d — asks to archive.
- esc / tab — back to Search.  
  Shortcuts (?)
- What it shows — general keys plus per-row meanings, or the current previesc / ? / q — close.
  ssages log (!)
  What it shows — every mess:MM:SS.
- esc — close.  
  Brag
- List rows — years (expand or collapse), year review, months and weeks, eshowing "View your brag" o
- List enter — opens a saved brag, asks to generate a missing one, or saysperiod isn't over yet.
- View enter — edit. View b — brag again (rewri
- View ctrl+y — copy. View esc — back.
- Confirm y / enter — collects the facts and has Claude write the summary background, shown in Jobs.
- Editor ctrl+o — saves (shows an error if the brag doesn't parse). Editor ctrl+y — copy.
- Editor esc — cancel.  
  Automation
- Confirm y / enter — starts the draft or create step in the background anto the Draft tab.
- After it finishes — tags the note "#jira" etc.; shows "RE-AUTH NEEDED" o"AUTOMATION FAILED" in thewrong.
- Draft editor ctrl+o — saves the draft. Draft editor esc — cancel.
  tup wizard (first run or d
  Step 1 — "Do you work with
- Step 2 — where your repos live (default ~/Projects). Step 3 — which days you wotoggle, at least one).
- Step 4 — morning and evening summary times as HH:MM (tab switches; emptyoff).
- Step 5 — whether to show key hints (y / n). Step 6 — a tool check, theinstalls notifications; nwrites it without them. esc — closes without savin
  ttings form (,)
  Fields — Git, Repos, Work y hints, Notifications, plus a tool check. j/k — move between fields.
- tab — toggles a field, or starts editing a text field. space — toggles.
- h/l — choose the day under the cursor. enter — validates, saves, cations, and restarts the app.
- esc — closes; asks "Discard changes?" if anything changed. While editing a text fieldter keeps it and saves, escundoes the edit.  
  Missing note file
- When it appears — saving a note whose file was deleted shows "file is goit again?"
- y / enter — recreates the file. n / esc — drops the note f
  I
  digest / digest tui — opens on first launch.
- digest repo-sync [--root] [--dry-run] — syncs your git repos. digest janitor [--root] [-mp files and removes stalereview clones. digest branch-reaper [--rotale local branches.
- digest pr-review --url — fresh clone plus a Claude review of the PR. digest brag --week|--monthenerates a brag.
- digest automation --note --name --phase draft|create — drafts or createsticket or doc.
- digest notify-due — sends due reminders and the morning/evening summarielaunchd).
- digest install — interactive installer for tools, notifications and a shdigest install notificatiofications — manage the launchd agent. digest setup — runs the se
- digest export config — writes the default config (backing up an existingdigest migrate — renames nthe TUI first).
- digest doctor — checks your tools and notification setup. --config — use another con
- --dry-run — loads the config without writing anything.  
  Things that may not work the way you'd expect
- Pending PR keys — y, d, r and o on a dashboard PR row do nothing unless show_key_hints is on; insiays work.
- Job preview with no output — its text says "[r]… dry-run, [Enter] to execute", but r runs the job and d dry-r
- ctrl+y on My PR and run previews — copies an empty string.
