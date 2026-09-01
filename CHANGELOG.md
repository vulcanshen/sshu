# Changelog

## [Unreleased]

The big rework. Three sections of it were specified together and land
together: the tab system, the ssh tab, and process cleanup.

### Changed

- **Tabs live on Alt chords now** — `[Alt-P]reference` / `[Alt-F]ileTransfer`
  / `[Alt-S]sh`, and the bracket still prints the key as pressed (the capital
  means shift). The chords work from inside a PTY — the whole reason they
  moved off the digits — and outside one the unshifted chord answers too.
  Every bare digit `1`–`9` now addresses a panel of the current tab, numbered
  from 1, so the sftp panels are `[1]`–`[4]` and nothing starts at 4 any more.
  Minimum width rises to 32 columns (the short strip).
- **The hosts tab became `[Alt-P]reference`** — a side nav over three
  sections: hosts (the old table, exactly where it was), **credentials**, and
  **logs**. The nav cursor IS the choice; Enter moves the keyboard over; the
  tab still opens on the hosts table.
- **The ssh tab became a grid.** Any number of sessions can hold a cell at
  once: `Tab` on the list toggles a session's cell, `Enter` shows one and
  hands it the keyboard, `Alt+1`–`9` jump between cells, `Alt+Esc` comes
  back. A layout strip arranges the grid — horizontal, vertical, or a custom
  columns × rows (Enter asks; overflow grows rows rather than losing a cell).
  Each cell's remote is told its own size, only when it actually changes; a
  dead cell leaves the grid and the keyboard never silently lands in another
  remote. List rows lead with a display column: a monitor glyph for a session
  with a cell, a struck-through one without.
- **The `!` popup is gone.** The app log is the third preference section;
  landing it on screen is what marks its errors read, and the nav row and
  footer carry the unread count until then.
- **The pick-a-value form fields changed hands.** On IdentityFile (and the
  new Credential field): Enter on the empty field opens the chooser, Enter on
  a filled one moves on, Backspace clears the whole line. Tab is "next" on
  every field again.

### Added

- **Credentials** — reusable identities in `credentials.yaml` (same 0600 +
  warning-header treatment as hosts.yaml). A credential is user + auth as ONE
  package; a host says `auth: credential` and takes it wholesale, its own
  User row going dark. Resolution happens at the doors: the connect
  confirmation shows who the session will run as and a dangling reference
  fails right there; the hosts table shows the credential's user (`?` when
  it is gone) and the credential's name in the auth column. Deleting or
  renaming a credential counts the hosts still naming it.
- **The app log persists** to `applogs.yaml` — a bare YAML list, appended one
  entry at a time, self-trimming past 1 MiB, 0600 with a warning header
  (failed connections put other machines' words in it). The tail is loaded
  back on start, shown but not unread. This reverses 0.1.0's "nothing about
  a session is written to disk", by explicit decision, with those mitigations
  in its place. And it records more than failures now: host/credential
  changes, connections opening and ending, sftp dials, transfer endings,
  edit write-backs.
- **No exit leaves an orphan.** Every child ssh leads its own PTY session,
  so no signal reaches it on its own. A process registry knows them all, and
  every way out — including outside SIGINT/SIGTERM (which end the loop
  without the model's quit path) and SIGHUP (the terminal window closing) —
  kills them on the way. Verified end to end against the demo container.

## [0.1.0] — 2026-09-01

First release. sshu is a terminal front end for ssh and sftp, and the third
member of the `u`-family — a parallel implementation of
[VTP](https://github.com/vulcanshen/thoughts/blob/main/vtp.md) alongside
[kbu](https://github.com/vulcanshen/kbu) and
[filu](https://github.com/vulcanshen/filu), not a derivative of either.

### Added

**Everywhere**

- **Five keys drive the whole app** — `Tab` moves to the next panel of this tab,
  `Enter` commits, `Esc` backs out one level, `Space` opens the contextual menu
  for whatever has focus, `?` opens the global help. `1`–`3` switch tab, `4`–`7`
  jump straight to a panel.
- **Every letter hotkey is also a row in that panel's `Space` menu**, because
  both are generated from one table per tab. A hotkey that is not in the menu
  cannot exist, and the bracket prints the key exactly as you press it — `[A]dd`
  is shift+A, `[t]ransfer` is a bare `t`, and nothing else fires.
- **Menus in two regions** — `item operation` for the row under the cursor,
  `panel operation` for the side as a whole. A menu with only one region stays
  flat rather than growing a header over a single group.
- One navigation vocabulary for every list: `j`/`k` wrap, `u`/`d` and
  `Ctrl+U`/`Ctrl+D` move half a page and clamp, `gg`/`G` jump to the ends,
  arrows are synonyms. Those letters are reserved — no action may claim one.
- **Frame stability** — every rendered line is exactly the terminal width, at
  every size, with any content. Wide characters from a remote, Nerd Font glyphs
  and CJK filenames are measured rather than assumed, and a test checks it
  across sizes, focus states and data.

**`[1]` hosts**

- A table over `hosts.yaml` — name, user, host, port, auth — shedding columns as
  the terminal narrows.
- `[A]dd` / `[E]dit` open a form with live validation; `[D]elete` and `Enter`
  (connect) ask first.
- `Tab` on the IdentityFile field opens a fuzzy file picker that shows
  permissions and flags a key other people can read.
- `/` searches name, user, host and port at once — fuzzy, ranked best-first, and
  deliberately **not** the auth column, which is a glyph rather than a word.

**`[2]` sftp**

- **Two independent filesystems side by side.** Either end can be this machine
  or a saved host and both can be remote, so upload, download and
  remote-to-remote are one code path behind a single `FS` interface.
- Per-side marks. A mark is an absolute path, so it follows a rename and is
  dropped when the file is deleted.
- **`/` searches the whole subtree** — breadth-first, because over SFTP every
  directory is a round trip and what is near should arrive first. Streaming,
  cancellable, capped, and drawn in place: a result is an ordinary row, so
  marking and transferring it needs nothing new.
- **A real transfer engine** — the whole plan is computed before anything is
  written, so the progress denominator is right from the first frame and
  overwrites are asked about once, up front. Per-job cancel; a cancelled or
  failed file is removed rather than left looking complete.
- `[r]ename` in place, `[x]` to delete the row and `[X]` to delete every mark,
  both after a confirm that names how many and on which host. Recursive delete
  walks with `Lstat`, so a symlink to a directory is unlinked instead of having
  its target emptied.
- **`[A]dd`** makes a file or a directory from one box — a trailing `/` is the
  whole difference, and the Enter verb changes as you type it so you always know
  which one you are about to make.
- **`[v]iew`** reads the item under the cursor without fetching it: text
  syntax-highlighted with line numbers, a binary as an xxd-style hex dump, a
  directory as one level of its listing. Capped at 64 KiB, because on a remote
  side every one of those bytes crosses the network. Escape sequences are
  stripped — those bytes come off someone else's machine.
- **`[e]dit`** opens the item in `$VISUAL` / `$EDITOR` (`vi` only as a floor,
  never a dependency) inside the embedded terminal. A remote file is fetched,
  edited and written back; a local one is edited where it lives, so its inode
  and every hard link to it survive. Nothing is written back unless the content
  actually changed, the write lands atomically, and a file somebody else changed
  while you had it open is never overwritten without asking.
- **Directories stay current cheaply** — SFTP has no change notification, so
  sshu stats the directory and compares its mtime, re-listing only when it
  moves, and only while the tab is on screen.
- A spinner and an elapsed count while a side is connecting, because a dial can
  take its full timeout and a still frame is what stuck looks like.

**`[3]` ssh**

- **Many concurrent sessions**, each a real `ssh` in an embedded PTY, one shown
  at a time. Shelling out to `ssh` means `ssh_config`, `ProxyJump`, agent
  forwarding and host-key handling all keep working.
- `[5]` takes the whole tab while the remote holds the keyboard; **`Alt+Esc`**
  takes it back.
- Rows read `<user>@<host>` with the port at the right edge, and the session on
  screen is green.
- `[C]lose` and `[D]uplicate`, both after a confirm.
- A spinner while a connection is being made, because ssh prints nothing at all
  while it waits for TCP and an empty terminal looks exactly like a frozen app.
  Keys are not forwarded during that wait: ssh is not reading its stdin, so they
  would be delivered to the remote shell whenever it finally arrived.
- A failure says **what ssh said** — `Connection refused`, not `disconnected` —
  keeps saying it in `[5]` instead of going blank, and raises a toast. A clean
  `exit` says nothing, because that is what you asked for.

**The app log**

- **`!` opens a record of what happened while you were not looking**, and `!`
  closes it again. Newest first, no cursor, nothing in it can be acted on.
- It holds **the whole final screen** of a failed connection, not one line: a
  refused connection is one line, but a host key mismatch is fifteen and the
  fingerprint you need is in the middle of them. Entries are capped at 40 lines.
- The footer discloses the key and counts what you have not read — `! 2 errors`.
- A toast and the log are two jobs, not two options: the toast is "this just
  happened" and vanishing is its function; the log is the part you can go back
  to.

**Data**

- One hand-editable `hosts.yaml`, resolved through `$SSHU_CONFIG` →
  `$XDG_CONFIG_HOME/sshu` → `os.UserConfigDir()/sshu`. Writes are atomic and
  re-assert mode `0600` every time.
- An optional `config.yaml` beside it, which sshu **never writes** — a file you
  have edited is never reformatted and your comments survive. `connect_timeout`
  (seconds, default 15) bounds one connection attempt: tab `[3]` hands it to ssh
  as `-o ConnectTimeout` so ssh produces its own message, and tab `[2]` uses it
  to bound its dial. A value outside 1–600 is treated as a typo; a file that
  cannot be parsed does not stop sshu from starting, and says so in the app log.

### Security

- **Passwords are stored in plaintext** in `hosts.yaml`. This is a deliberate
  trade, and the README says so at length. The mitigations: `0600` re-asserted
  on every write, a warning header in the file, the password never rendered, and
  `SSH_ASKPASS` used so the secret **never enters a child process's
  environment** and never appears in `ps`.
- **`[2]` refuses an unknown host key** rather than waving it through, and
  refuses a **changed** one outright rather than offering it as a question. To
  accept a new host, connect once through `[3]` — that is OpenSSH's prompt, with
  OpenSSH's fingerprint.
- The filename handed to `$EDITOR` is a positional parameter and never enters
  the shell script that starts it: the name comes off a remote machine, and one
  containing `;` must arrive as an argument rather than as a command.
- Content read from a remote is sanitised before it is drawn. A file full of
  escape sequences would otherwise repaint the popup, the panels behind it, or
  the terminal.
- Nothing about a session is written to disk. The final screen can hold anything
  the remote printed, and a connection log is not worth the leak.

### Known gaps

- No release binary, Homebrew tap or install script — build from source.
- `[2]` cannot interactively accept an unknown host key; use `[3]` once.
- Encrypted private keys are reported plainly but not usable.
- No content search on a remote: that would mean running a command on the far
  end, which this tab deliberately does not do.
- No mouse support, no `fsnotify` reload of `hosts.yaml`, no session
  persistence, no keychain-backed password storage.

[Unreleased]: https://github.com/vulcanshen/sshu/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/vulcanshen/sshu/releases/tag/v0.1.0
