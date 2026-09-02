# Changelog

## [1.1.0] — 2026-09-02

The file transfer tab answers for itself while it works. A side can be let go
of as well as picked (`[D]isconnect`), a directory can be re-read on demand
(`[R]efresh`), and a transfer in flight is visible in three places at once: the
summary spins, the file being written spins in its own mark column and refuses
to be marked, and the two actions that would pull the filesystem out from under
it dim rather than vanish. The host form also stops charging two extra Enters
for choosing a credential.

Changes since 1.0.0:

### Added

- **`[D]isconnect` on the file transfer tab's `[1]` / `[3]`** — `[S]elect host`
  was one-way: you could point a side at a host but never let it go without
  quitting sshu. `D` hands the side back the state it had before a host was
  picked — no filesystem, no listing, no marks, all of which belonged to the
  host that is leaving. It is refused while a transfer is running (both sides
  are on every transfer, so closing either one breaks it) and says to stop it
  in `[P]rogress` instead.

- **`[R]efresh` on the file transfer tab's `[1]` / `[3]`** — re-reads the
  directory now, unconditionally. The background watch re-lists every couple of
  seconds but only when the directory's own timestamp moved, and a timestamp is
  not a promise: a filesystem can change underneath one without touching it.
  This is the key for the doubt the poll cannot settle. It stays live during a
  transfer — reading is the one thing that is still safe while bytes move — and
  it reports the count, because a refresh that changes nothing otherwise looks
  exactly like a key that did nothing.
- **A file still being written into shows a spinner in its mark column, and
  cannot be marked** — a transfer creates the destination file and then fills
  it, so the row is in the listing before the bytes are. Marking it would
  promise the path is a thing you can act on; send that mark onward with `[T]`
  and what lands at the far end is a truncation nobody was told about. The
  spinner takes the mark cell rather than a column of its own — the cell is
  free by construction, and a row that grew a column mid-transfer would shift
  every name beside it. A directory the bytes are landing in counts too: copy a
  whole tree and the directory is the row you can see. Both clear, and the
  listing re-reads, the moment the job ends.
- **A Space menu row can now be *disabled*** — dimmed in place instead of
  removed. Leaving an action out is how the menu says "this does not apply
  here"; dimming it says "this belongs here, but not this second". The cursor
  still lands on a dimmed row, wearing the quieter bar, because that is how you
  find out why it is dim.

### Changed

- **`Space` on a file transfer side with no host opens the host list itself** —
  it used to draw a menu whose only row was `[S]elect host`, which answered
  nothing and pushed the answer one keystroke further away. The condition is
  the side having no host, so it holds on that side's marks panel too. (A menu
  with *nothing* to run still opens — "nothing" is an answer that has to be
  said out loud; one concrete action is an answer best given by doing it.)
- **Changing a side's filesystem is frozen while a transfer is running** —
  `[S]elect host` and `[D]isconnect` both swap out the filesystem under a side,
  and every transfer in this tab has both sides on it, so either one would
  break a copy in flight. Both rows dim, the keys refuse with a line pointing
  at `[P]rogress`, and the refusal leaves the menu standing — the row it
  refused is still on screen, still explaining itself.
- **The transfer summary spins** — `⠋ 󰁥 3/7 · 42%`. A percentage can sit at
  99% for a long time on a big file, and a number that is not moving is what
  stuck looks like. It rides the 120 ms tick that already repaints the line.
- **`Enter` on a filled IdentityFile or Credential field now saves** — it used
  to step to the next field, so choosing a credential cost two more Enters and
  a lap back round to Name before the host could be written. The empty field
  still spends `Enter` on the chooser, because an empty field has nothing else
  for `Enter` to mean; a filled one is just a field, and `Enter` means on it
  what it means everywhere else on the form. `Backspace` still clears the whole
  line, and `Tab` / `↓` are still how you go to the next field.

## [1.0.0] — 2026-09-02

First stable release. The surface is settled: three Alt-chord tabs
(preference / file transfer / ssh), the grouped `[1] sshu` nav, reusable
credentials, an app log that survives the process, two-sided sftp with a
transfer queue, and an ssh terminal grid — with no way out that leaves an
orphan behind. 300+ tests, `make check` green and `-race` clean.

Changes since 0.1.1:

### Added

- **`[C]lear logs`** — the app log's Space menu had nothing to offer but its
  own description; it now carries one action. It asks first, and clearing
  takes `applogs.yaml` with it (the file goes first — a panel emptied while
  the file survives fills straight back up on the next start). An empty log
  offers neither the row nor the key.

### Fixed

- **A Space menu with nothing to run drew a clipped stub** — a menu whose rows
  are all description (an empty log, the nav) measured itself at zero, so the
  box came out title-width with its own words cut (`nothing reco…`) and its
  legend cut mid-key. Headers and the legend are measured like any other
  content now, and a menu with nothing to run offers only `Esc close`.

### Changed

- **The `[1] sshu` nav dims while the content panel holds the keyboard** —
  headers, items and the cursor bar all recede together, so the nav reads as
  the legend it is rather than competing with the panel being used. The
  cursor stays a bar, one register down, and the unread-error count is the
  one thing that does not dim.

## [0.1.1] — 2026-09-01

### Added

- **`[1] sshu` nav, in groups** — the preference nav is retitled from
  `[1] Resources` and grouped kbu-style: **SSH** (Hosts, Credentials) and
  **Events** (Logs). Headers wear the structure blue, are skipped by the
  cursor, and the section names are capitalized. (An Operation group —
  Export / Import of a `.sshu` config bundle — is implemented and tested
  behind a mask, waiting on its final design.)
- **Splash byline** — the easter egg signs off with `developed by` /
  `vulcan.shen.2304@gmail.com` on their own lines, above the Esc hint.

### Fixed

- On a terminal narrower than 60 columns, an **empty app log** drew a panel
  shorter than the frame (the log's empty state returned fewer rows than the
  panel is tall, and the narrow layout has no neighbouring panel to prop it
  up).

## [0.1.0] — 2026-09-01

First public release.

**sshu** is a terminal front end for ssh and sftp — the ssh-domain member of
the u-family ([kbu](https://github.com/vulcanshen/kbu),
[filu](https://github.com/vulcanshen/filu)), driven by
`Tab` / `Enter` / `Esc` / `Space` / `?`.

### Highlights

- **Three tabs on Alt chords** — `[Alt] ❯ [p]reference ❯ [f]ile transfer ❯
  [s]sh`: an always-lit `[Alt]` lead spells the chord together with the lit
  tab. The chords work from inside a PTY (shifted spelling there), and every
  bare digit `1`–`9` addresses a panel of the current tab.
- **Preference** — hosts, credentials and the app log under one nav. Hosts
  are a table over `hosts.yaml` with add/edit forms, live validation and
  ranked search. Credentials are reusable identities: `auth: credential`
  takes user + auth wholesale, resolved and shown at the connect
  confirmation. The app log persists to `applogs.yaml` — connections,
  transfers, edits, and failures with the remote's actual words.
- **File transfer** — two independent sides, each this machine or a saved
  host, so upload, download and remote-to-remote are one operation. Marks
  (`[a]ppend` / `[c]lear` / `[C]lear all`), recursive breadth-first subtree
  search drawn in place, view (`v`) and edit (`e`) without leaving, and a
  transfer engine that plans before it writes, asks about overwrites once,
  and cancels per job. While bytes move, the top-right summary reports in
  green and the rule under the tab row doubles as a progress bar — on every
  tab.
- **ssh** — a grid of live terminals, each a real `ssh` on its own PTY.
  `Tab` toggles a session's cell, `Enter` enters, holding `Alt` the arrows
  steer between cells, `Alt+Esc` takes the keyboard back; layouts are
  horizontal / vertical / custom rows × columns. The list cursor lights its
  session's cell on the grid.
- **No exit leaves an orphan** — `q`, `Ctrl+C`, outside SIGINT/SIGTERM,
  even the terminal window closing (SIGHUP) kill every child ssh on the way
  out.
- **Security posture** — every config write is atomic and re-asserts
  `0600`; passwords are never rendered and reach ssh via `SSH_ASKPASS`
  (never a child's environment, never `ps`); the sftp side refuses unknown
  and changed host keys outright.
- **Frame stability** — every rendered line is exactly the terminal width,
  at every size, with any content; wide glyphs and CJK are measured, not
  assumed.

Install via Homebrew (`brew install vulcanshen/tap/sshu`), the install
script, or `go install`. macOS / Linux only; a Nerd Font is required.
