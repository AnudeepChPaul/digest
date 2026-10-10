Everywhere

- ctrl+c — works in any view: each press restarts a 2s window, the 3rd press within it stops git sync and exits, and any other key in between resets the count.
- ctrl+c warning — the dashboard footer shows "ctrl+c N more"; other popups show "ctrl+c N more to quit" on their bottom line.
- Narrow terminal — narrower than the header (banner, version and date, about 45 to 60 columns, never under 40) the screen only says "Terminal window is too small."
- Scroll keys in preview and brag view — j/k/arrows, pgup/pgdown, ctrl+d/ctrl+u.
- ctrl+d/ctrl+u in the note, brag and automation editors — move half a page; pgdown/pgup move a full page; forward-delete is the delete key only.
- tab in the note, brag and automation editors — inserts four spaces.
- esc in the note, brag and automation editors — closes straight away when nothing changed; otherwise asks "Discard changes?" (y discards, n / esc keeps editing).
- Errors never open a popup — they appear in the header as "✗ source: text" and stay in the message log.
- Errors on other screens — popups, previews, Search and the editors also show the error as a one-line "✗ source: text" notice under their footer; the next key press clears it.
- ctrl+y — copying shows "✓ copied" in the header; if the clipboard fails it shows "✗ clipboard: reason"; with nothing to copy it does nothing.
- Opening links — if the browser can't open a link, the header shows "✗ browser: reason".

Dashboard: what you see

- Header — the "digest" banner plays a wave animation once at startup.
- Header, top line — "🔥 N-day streak" (work days in a row with notes closed) and "N closed this week".
- Header, top right — the newest message: a spinner with "git syncing", "✓ git synced" (fades after 4s), or "✗ source: error" (stays until esc).
- Header, middle line — version, today's date, and a yellow "Last week (Wnn) isn't bragged — press b" when last week has no brag.
- Yesterday — done notes from your last working day (work_days in the config) before the day you're viewing; non-work days in between are skipped, so go to them with p to see their notes; titled "Y E S T E R D A Y · date" or the spaced weekday name.
- Today — "T O D A Y · date" (or the weekday when viewing another day) with a "N jobs ◆" badge on the right.
- Today: Pending, Carried Over (n) — active notes created before the day you're viewing.
- Today: Added Today (n) — active notes created on the day you're viewing.
- Today: Closed Today (n) — notes marked done on the day you're viewing.
- G I T strip — two columns, the previous day and the day you're viewing, with one row per repo: "N assigned · N reviewed · N commits".
- M Y P R ( S ) — your open PRs grouped by repo: number, branch, "(draft)", files, age, an approved glyph, and CI (✓ ✗ ◌ ·).
- Pending Git Actions — PRs waiting for your review, grouped by repo, with the sort hint "s Updated|Created w ↓|↑ m scope".
- PR row tags — size ±N, age (red when stale), state (pending, reviewing, rec:X, approved, changes requested…), and first-review/re-review and direct/team
  icons.
- Jobs — configured jobs (◆, or ✔ after a clean dry run), review runs, brag runs and
  running or failed automation runs.
- Note tags — "@notify:interval", "#draft" or "#jira" (automation), age (HH:MM,
  weekday, or yellow "Nd ago" when carried over), and the source (#manual, #pr-review…).
- Automated tag — shows only for known kinds: manual, my-pr, pr-review, or the automation_type of a configured automation; any other "automated:" value shows no tag.
- Notes in memory — the dashboard keeps only active notes and the notes finished on the day you're viewing and the previous working day.
- Corrupt notes — a note file that doesn't parse shows "✗ store: file: reason" in the header; the other notes still load.
- Note status — a missing or unknown status loads as active.
- When tags show — on the selected row only, unless show_tags: always; PR notes and
  the notify tag always show.
- Key hint pill — with show_key_hints on, a pill under the selected row lists its
  keys after 250ms of no typing.
- j/k order — Yesterday, Carried, Added, Closed, git repos, My PRs, Pending PRs,
  jobs, review runs, brag runs, automation runs.
- Startup selection — selection_default: notes_today or notes_yesterday picks where
  the cursor starts; otherwise it starts on the first row.
- Startup git data — shown from the cache; a sync runs when the cache is stale or
  has no data for the viewed or previous day. My PRs are always re-fetched on startup.
- Invalid git roots — if the configured repo roots are invalid at startup, git is
  turned off and the error shows in the header.
- Auto-sync — runs a git sync every git_auto_sync_interval seconds (default 600).
- Day sync — p, n and t start a git sync for the new day 2s after the last press.
- PR alerts — after a sync, macOS notifications fire for approved, changes
  requested, merged, closed, new comments, review requested and re-review, plus "PR opened" for a newly seen PR of yours. Every sync alerts, the first one too, on PRs that are new or whose state changed; unchanged PRs stay quiet.
- Unreadable PR status file — if cache/pr-status.json is corrupt, the header shows the error once and the file starts fresh; that sync alerts like a first sync.
- Automatic PR review notes — PRs you review create or update "#pr-review" notes; approved ones are closed, and they reopen if review is requested again.
- Automatic My PR notes — each of your open PRs gets a "NewPR:" note with an "## Updates" log, closed with "Merged" or "Closed".
- Corrupt my-prs-seen.json — it is rebuilt from your open PRs and their existing notes, without duplicate notes.
- Reminder cleanup — reminders are removed when their note is done or archived.
- Git turned off — the strip and pending section are hidden, and g c s w m
  stop working.

Dashboard: keys

- enter on a note — opens the preview and the editor; saving returns you to the preview.
- enter on a pending PR or My PR — opens it in the browser.
- enter on a job or run — opens its preview (inside a job preview enter asks to run an idle job).
- enter on a git repo — opens its details, like tab.
- space — marks the note done or active and saves it.
- a — opens the editor for a new note dated the day you're viewing.
- i — edits the selected note's summary in place.
- @ or . — opens the action menu (active notes only).
- o — opens the note's link, or a link menu when it has several (shown only if the
  note has URLs).
- d on a note — asks to archive it; on a note with no file yet it asks "This note doesn't exist. Delete?" and drops it from the list on yes.
- Row keys (enter, tab, space, i, d, r, @, .) — not bound when the dashboard has no rows.
- tab — opens the preview; on a git repo it opens the git details instead.
- t — jumps to today and loads its notes from disk.
- p / n — moves to the previous or next day (n goes at most one day past today) and loads that day's and the previous working day's notes from disk.
- ctrl+e — opens the Archive.
- ctrl+r (hidden) — reloads the notes of the day you're viewing.
- g — starts a git sync now, replacing any sync already running.
- c — refreshes local commits only; not bound when daily commits are off.
- r on a job — asks to run it; on a finished or failed run it asks to retry; r does nothing on other rows except pending PRs.
- j/k/arrows — move the selection.
- ctrl+d/ctrl+u — move half a page. pgdown/pgup — move a full page.
- / — opens Search.
- b — opens Brag without refreshing anything; when last week isn't bragged it selects that week.
- ? — opens Shortcuts.
- , (hidden) — opens the Settings form.
- ! (hidden) — opens the Messages log.
- s (hidden) — switches the pending-PR sort between Updated and Created; saved.
- w (hidden) — switches the pending-PR sort between descending and ascending; saved.
- m (hidden) — shows only PRs that ask for you by name; not saved.
- esc (hidden) — hides header errors and git sync failures; does nothing when there are none.

Dashboard: keys on specific rows

- Pending PR y — asks to approve (only when show_key_hints is on).
- Pending PR d — (only when show_key_hints is on) stops a running review; on a reviewed PR, asks to request changes with every finding (no selection needed); otherwise opens a comment box.
- Pending PR r — asks to start a Claude review, even when show_key_hints is off.
- Pending PR o — opens the review clone in nvim (only once a clone exists, and only when show_key_hints is on); outside tmux the header says "Not inside tmux" and the clone folder opens in Finder.
- Job r — asks to run it; during its dry run it asks "Dry run in progress. Stop it and run the job?" and kills the dry run first.
- Job d — asks, then starts a background dry run when idle; asks to abort when running; does nothing while its dry run is in flight. Jobs without a dry-run-command have no d dry run.
- Stale job d — a job whose pid file remains but whose process is gone asks "Clear its stale state?" and removes the pid file.
- Review run d — asks to stop it when running; dismisses it straight away when finished or failed.
- Brag run d — asks to stop it when running (yes stops every running brag run); dismisses it straight away when finished or failed.
- Automation run d — asks to stop it when running; dismisses it when failed (failed runs stay under Jobs until then).
- Run r — on a finished or failed review, brag or automation run, asks "Run again?" and starts it again.
- Stopping a run — kills its process group, removes its pid file and marks it failed; it stays under Jobs as failed until d dismisses it.

Inline edit

- enter — saves the trimmed summary; with the summary cleared it asks "Delete this note?": yes archives it like d, no keeps the original text.
- esc — cancels.
- Other keys — type into the summary.

Notify input (from @notify)

- Typing — accepts digits followed by m, h or d; an "@notify" or "@notify:" prefix is accepted too.
- Refused keys — a key that would make the interval 0 or more than 3d does nothing (4d, 73h and 4321m stop at the key that goes over).
- enter / y — saves the reminder: empty means 1h, the maximum is 3d, and no unit
  means hours.
- Save failure — the input stays open and shows the error.
- Deleted note — if the note is gone when you save, the input closes and the header shows "✗ notify: note no longer exists".
- The saved reminder opens the note's PR URL, or your terminal.
- esc — cancels. Saving, cancelling and @notify:off return to where the menu was opened, the preview included.
- Delivery — digest notify-due sends reminders via launchd every minute.

Action menu (@ / .)

- @notify — opens the reminder input. On a note that already has a reminder it is prefilled with the current interval, and saving rewrites the reminder with the new interval, counting from now.
- @notify:off — removes the reminder; listed first, followed by @notify, when the note has one.
- Automation items — jira ("ticket"), confluence ("confluence page"), google doc,
  google calendar ("calendar event", "schedule a meeting"); listed when the note matches the automation and hasn't been automated by a known kind, even while an automation runs.
- Automation chosen — asks to draft it.
- Ordering — @notify:off then @notify first on a note with a reminder, then the order in action_menu_order in .app.state.json; items it doesn't list keep their default order after it.
- j/k — move.
- enter / y — choose.
- esc / n — cancel.

Link menu

- enter — opens the link in the browser.
- j/k — move.
- esc / n — cancel.

Preview (all kinds)

- p / n — moves to the previous or next dashboard row inside the preview; landing on a git repo opens its details, landing on a running job starts its live log.
- h/l/←/→ — do nothing.
- ? — opens Shortcuts.
- esc (or tab where tabs don't apply) — closes.

Note preview

- enter — edits the note. space — marks it done or active.
- @ / . — opens actions. o — opens links.
- d — asks to archive, then moves to the next row. ctrl+y — copies the summary and body.

Note preview: Draft tab (when the note has an automation run)

- tab — switches between the Details and Draft tabs. esc — closes.
- enter — edits the draft.
- r — asks to create the ticket or doc from the draft. d — asks, then deletes the draft.
- space, @ / . and o — act on the note, as on the Details tab.
- ctrl+y — copies the draft.
- p / n — moves to the previous or next row.

Pending PR preview

- Details tab — author, CI, wait time, size, Jira link, code owners, your review, files, and recent changes to those files.
- Re-reviews — the Details tab also shows commits and files changed since your last review.
- History failure — if the recent changes can't be read, the Details tab says "Couldn't read history: reason".
- Review tab — Claude's recommendation, findings by severity with checkboxes, or the review's progress or failure.
- tab — switches tabs. y / a — asks to approve.
- d — stops the review, or requests changes with the selected findings, or opens a comment box.
- r — asks to start a review. o — opens the clone in a new nvim window (inside tmux only).
- space — selects the finding under the cursor. ctrl+a — selects or clears all findings.
- j/k — move between findings. enter — posts selected findings as a comment review; with none selected, opens the PR in the browser. ctrl+y — copies the title, URL and findings.

My PR preview

- What it shows — title, branch, files, created, CI, review decision, reviewers and description. enter — opens the PR.
- ctrl+y — copies the whole preview as plain text, including the lines scrolled out of view.

Job preview

- What it shows — the live log, dry-run output, or the finished log (last 400 lines); it refreshes every 200ms while the job runs. With no output it says "(d) dry-run (r) run", or only "(r) run" when the job has no dry-run-command.
- r / enter — asks to run (the footer shows "r|⏎ run"); enter works only while the job is idle. d — asks, then dry-runs (only with a dry-run-command), or asks to stop when running.
- ctrl+y — copies the log; with no output it copies nothing.

Review / brag / automation run preview

- What it shows — the run's log tail, which updates as the run writes to it. d — stop or dismiss; after a stop the preview stays on the failed run.
- r — on a finished or failed run, asks to retry. There is no enter.
- ctrl+y — copies the whole preview as plain text, including the lines scrolled out of view.

Git details popup

- tab — cycles the filter: All, Reviewed, Assigned, Commits. enter — opens the item in the browser.
- j/k — move. p / n — moves to the previous or next dashboard row and opens its preview. esc — close.

Confirm popups (run / dry run / abort / stale / retry / stop / archive / restore / delete)

- y / enter on run — starts the job in the background; if it can't start, the error shows inside the confirm, which stays open.
- y / enter on dry run — starts the dry run in the background.
- y / enter on abort — sends SIGTERM, then SIGKILL after 3s, and logs "[JOB ABORTED BY USER]".
- y / enter on stale — removes the job's leftover pid file.
- y / enter on retry — starts the run again.
- y / enter on stop — stops the automation run, or every running brag run. y / enter on archive — archives the note.
- y / enter on restore (from the Archive) — restores the notes as active.
- y / enter on delete (from the Archive) — deletes the note files permanently; a note whose file is already gone counts as deleted. n / esc — cancel.
- Draft cleanup on delete — if a deleted note's automation draft can't be removed, the header shows "✗ notes: deleted, but couldn't remove draft · reason".

Review confirm

- y / enter — submits the approve, request-changes or comment review to GitHub, updates the review note, then resyncs. esc / n — cancel.

Start / stop review confirm

- y / enter — starts a Claude review in the background (fresh clone, install, review), which appears in Jobs; or stops the running one. esc / n — cancel.

Reject comment box

- ctrl+s — submits the request-changes review straight away, with no further confirm; refuses if the comment is empty. ctrl+y — copies the comment. esc — cancel.
- Length — the comment has no line limit.

Archive

- space — selects or deselects a note for a multi-note action. u / enter — asks, then restores the selected notes (or the highlighted one) as active, dated the day you're viewing; the Archive stays open. d — asks to delete them permanently.
- Empty Archive — space, u, enter and d are not bound.
- j/k — move. ctrl+d/ctrl+u — move half a page. pgdown/pgup — move a full page. The list scrolls to keep the selection in view.
- esc / ctrl+e — close.
- Loading — the Archive and Search read every note from disk when they open; Brag does too until .app.state.json knows your first note.

Note editor

- Layout — the first line is the title; everything after it is the body.
- ctrl+o — saves; a blank first line means the first non-blank line becomes the title. An empty note, new or existing, is never saved; the editor shows "note is empty". ctrl+y — copies the text, trimmed.
- esc — cancel; asks "Discard changes?" when the text changed. ctrl+d/ctrl+u — move half a page. pgdown/pgup — move a full page. tab — inserts four spaces.

Search

- Blank query — shows no results.
- Plain words — must appear in the summary or body.
- tag:x — matches the note's source or subject. date:7d / 2w / 3m / 1y — notes created or updated in the last N days, weeks, months or years.
- date:DD-MM-YYYY — notes created or updated on that exact day. Scope — active and done notes, with matches highlighted.
- up/down — move. pgup/pgdn — page.
- ctrl+d/ctrl+u — half page.
- enter — opens the result like enter on the dashboard: the note's preview with the editor. tab — opens the note's normal preview.
- No results — enter and tab are not bound.
- From that preview — p / n move to the previous or next result; esc / tab go back to Search; archiving shows the next result, or goes back to Search when none is left.
- ctrl+e — exports the results to ~/Downloads/search-<timestamp>.csv and copies the path.
- esc — close.

Shortcuts (?)

- What it shows — general keys plus per-row meanings, or the current preview's keys. esc / ? — close.
- PR meanings — with show_key_hints off, the PR meanings of y, d, r and o are left out; with git off, every PR meaning is.

Messages log (!)

- What it shows — the last 50 messages, newest first, with their time as HH:MM:SS; an empty log shows nothing.
- Colours — errors red (✗), successes green (✓), progress violet; a git sync still running has an animated spinner.
- Long lines — trimmed to one line with "…".
- ! — shows the full log with every line wrapped; j/k/arrows, pgup/pgdown and ctrl+d/ctrl+u scroll it.
- esc — close.

Brag

- List rows — years (expand or collapse), year review, months and weeks, each showing "View your brag" or "Brag about this week?" (or "Bragging..." while it runs); a saved brag that can't be read shows "Brag about this week again? · unreadable".
- List enter — opens a saved brag, asks to generate a missing one, or says the period isn't over yet; on an unreadable brag it shows the error and asks to generate a fresh one.
- View enter — edit. View b — brag again (rewrites only the summary from the saved facts).
- View ctrl+y — copy. View esc — back.
- Confirm y / enter — collects the facts and has Claude write the summary in the background, shown in Jobs. Confirm n / esc — cancel.
- Editor ctrl+o — saves (shows an error if the brag doesn't parse). Editor ctrl+y — copy.
- Editor esc — cancel; asks "Discard changes?" when the text changed.

Automation

- Confirm y / enter — starts the draft or create step in the background and switches the preview to the Draft tab. Confirm n / esc — cancel.
- Start errors — shown in the header and as "AUTOMATION ERROR: reason" in the preview or draft editor; the next automation that starts clears them.
- After it finishes — tags the note "#jira" etc.; shows "RE-AUTH NEEDED" or "AUTOMATION FAILED" in the header when something went wrong.
- Failed runs — stay under Jobs until d dismisses them.
- Draft editor ctrl+o — saves the draft. Draft editor ctrl+y — copies it. Draft editor esc — cancel; asks "Discard changes?" when the text changed.

Setup wizard (first run or digest setup)

- Step 1 — "Do you work with git repos?" (y / enter yes, n no).
- Step 2 — where your repos live (default ~/Projects). Step 3 — which days you work (←/→ move, space toggle, at least one).
- Step 4 — morning and evening summary times as HH:MM (tab switches; empty turns one off).
- Step 5 — whether to show key hints (y / enter yes, n no). Step 6 — a tool check that can't be skipped; y / enter writes the config and installs notifications.
- ←/→ — on steps 1, 5 and 6, move to the previous or next step you've already reached (skipping the repos step when git is off); not on the work days or text steps.
- esc — closes; asks "Close setup without saving your changes?" when an answer changed (y discards, n / esc keeps going).
- Config file — the default config is written only when the wizard finishes; quitting earlier writes nothing.
- Errors — a failed save or notification install shows as a notice inside the wizard, which stays open so you can try again.
- Backup — an existing config.yaml is copied to config.yaml.bak once, on the first attempt; retries keep that first backup.

Settings form (,)

- Opening — , opens it even when digest was started without a config path, using the default config.yaml.
- Fields — Git, Repos, Work days, Morning, Evening, Key hints, Notifications, plus a tool check. j/k — move between fields.
- tab — toggles a field, or starts editing a text field. space — toggles.
- h/l/←/→ — choose the day under the cursor. enter — validates, saves, installs or removes notifications, and rebuilds the dashboard in-process; with nothing changed it does nothing.
- Saving — while it saves, a spinner shows and every key is ignored, ctrl+c included.
- esc — closes; asks "Close setup without saving your changes?" if anything changed. While editing a text field, tab keeps the value, enter keeps it and saves, esc stops editing, asking "Undo the change you're typing?" if you typed anything.
- Discard / undo prompts — y discards, n / esc keeps editing.

Missing note file

- When it appears — saving a note whose file was deleted shows "file is gone, create it again?"
- y / enter — recreates the file. n / esc — drops the note from the list and stays where you were (the preview included).

Jobs in config.yaml

- Built-in jobs — branch-reaper, janitor and repo-sync, with their commands and dry-run commands, are built into digest; a job missing from config.yaml (or no jobs section) runs with digest's defaults. Names match ignoring case, spaces and underscores ("branch reaper" is branch-reaper).
- Built-in entries — only name and options; command or dry-run-command on one fails to load with "job <name>: command and dry-run-command are built in; only options can be set; run digest migrate".
- Custom jobs — any other name keeps its command and optional dry-run-command.
- options — every option reaches the job, from the dashboard or the shell, as a DIGEST_OPTION_<NAME> environment variable, with lists comma-joined; built-in jobs ignore options they don't know. An option set in config.yaml replaces that option's default; the other defaults stay.
- roots — the directories a built-in job works on; defaults ~/Projects for branch-reaper and repo-sync and ~ for janitor; --root overrides it. Quote a lone ~ ("~"); a bare ~ in the list is read as ~ too.
- branch-reaper grace_days — default 7; branches whose PR merged within that many days are kept.
- janitor grace_days — default 14; quarantine batches older than that are purged. It replaces the old top-level retention_days.
- janitor patterns — the file names to quarantine. It replaces the old top-level janitor_patterns.
- janitor no_quarantine — default false; true deletes matching files for good instead of quarantining them, like --no-quarantine.
- Old keys — digest migrate moves retention_days and janitor_patterns into the janitor job's options, adding a janitor entry with options only when there is none, and removes built-in job commands, turning their --root values into the roots option (from command, else dry-run-command) and renaming "branch reaper" / "repo sync" to branch-reaper / repo-sync. Custom jobs are left as they are.
- Paths — ~ and ~user expand; an unset $VAR or a missing home directory is an error, not an empty path.

Automations in config.yaml

- Built-ins — jira, confluence, google doc, google calendar and pr review come from the embedded config template; digest init writes them out with their full prompts.
- automation_type — every automation has one: jira ticket, confluence confluence, google doc doc, google calendar event, pr review reviewed. A built-in entry without one gets its built-in type; a custom automation without one fails to load with "automation <name>: automation_type is required".
- Draft then create — every automation except pr review needs draft_prompt and create_prompt; a missing one (after built-in defaults) fails to load with "automation <name>: draft_prompt and create_prompt are required". @notify is built into digest and isn't an automation.
- Create result — the create phase must return a kind equal to the automation_type, or the run fails with "<name> create: kind must be ...". The note gets a section headed by the type (Ticket, Confluence, Doc, Event).
- match — phrases that offer the automation in the action menu: jira ticket, jira, jira ticket; confluence confluence, confluence page, confluence doc; google doc google doc; google calendar calendar event, event, schedule a meeting, add to calendar.
- google calendar — uses the claude.ai Google Calendar connector (claude_ai_Google_Calendar); authenticate it once with /mcp in Claude Code. The draft stops with "NO TIME:" when the note names no date or start time; a start with no end lasts 30 minutes.
- Loading — the dashboard reads config.yaml once when it starts and serves automations from memory; digest automation reads config.yaml on every run.

CLI

- digest / digest tui — opens the dashboard; the setup wizard opens on first launch. A config that doesn't load shows in the header, not on stderr.
- digest repo-sync [--root] [--dry-run] — syncs your git repos. Without --root, each built-in job uses its roots option (janitor no longer defaults to the current directory). digest janitor [--root] [--dry-run] [--review-root] [--no-quarantine] — cleans up quarantined and temp files and removes review clones of merged or closed PRs, or idle for 7 days. digest branch-reaper [--root] [--dry-run] — prunes stale local branches.
- --root that doesn't exist — janitor and branch-reaper stop with an error. repo-sync reports each unusable root ("N roots unusable"), syncs the rest and exits 1; it fails only when no root is usable.
- repo-sync — the default branch comes from the remote, then origin/main or origin/master. Untracked files don't block a fast-forward. A repo with no remote is reported as no-remote.
- branch-reaper — a failed or unreadable gh query skips the repo and needs your action. A repo with tracked changes is skipped and only logged; untracked files don't block. The dry run says "would delete" when git branch -d would work, that is when the branch is merged into HEAD or its upstream.
- janitor — a quarantine batch that fails to purge counts as a failure. Review state is removed after 7 idle days, even when it has no metadata.
- janitor --no-quarantine — deletes matching files for good instead of quarantining them; the janitor job option no_quarantine: true does the same.
- digest pr-review --url — fresh clone plus a Claude review of the PR. digest brag --week|--month|--year [--regenerate] — generates a brag; --regenerate rewrites only the summary from the saved facts.
- digest automation --note --name --phase draft|create — drafts or creates the note's ticket or doc.
- digest notify-due — sends due reminders and the morning/evening summaries (run every minute by launchd).
- digest install — interactive installer for tools, notifications and a shortcut. digest install notifications / digest uninstall notifications — manage the launchd agent. digest setup — runs the setup wizard again.
- digest init [--force] — writes the default config.yaml into the config directory; refuses when one exists, and --force backs it up to config.yaml.bak first. digest init --automations backs up config.yaml and replaces only its automations section with the defaults and their full prompts, keeping everything else; custom automations are dropped, and with no config.yaml it writes the full default. It replaces digest export config. digest migrate — renames note files by status, gives older and PR notes timestamp ids, moves retention_days and janitor_patterns into the janitor job's options, adds automation_type to built-in automations that lack it, adds the google calendar automation when it's missing (prompts and match lists are left as they are), and sets first_note_created in .app.state.json from the earliest note, archived included (quit the TUI first). A missing .app.state.json is created with the streak too; an existing one keeps its other fields.
- digest migrate --dry-run — changes nothing and prints what it would do: the notes it would migrate, the config keys it would move, the automation types and google calendar entry it would add, the app state it would save or the first_note_created it would set, and the files it would lock.
- digest doctor — checks your tools and notification setup. --config — use another config file.
- --dry-run — loads the config without writing anything; digest --dry-run <command> works the same as digest <command> --dry-run.
- --dry-run refused — pr-review, brag, automation, init, install, install/uninstall notifications and notify-due print "<command> doesn't support dry-run" and exit 1.
- --dry-run ignored — tui and setup run as usual.
- -h / --help / help — prints the commands and flags.
- Unknown flag or command — prints the error and the usage, and exits 1.

Things that may not work the way you'd expect

- Pending PR keys — y, d and o on a dashboard PR row do nothing unless show_key_hints is on; r always works, and inside the preview they all work.
