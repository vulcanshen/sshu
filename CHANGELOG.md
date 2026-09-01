# Changelog

## [Unreleased]

### Added

- **`[1] sshu` nav, in three groups** — the preference nav is retitled from
  `[1] Resources` and grouped kbu-style: **SSH** (Hosts, Credentials),
  **Events** (Logs), **Operation** (Export, Import). Headers are decoration —
  the cursor skips them — and the section names are capitalized.
- **Export** — bundle `hosts.yaml` + `credentials.yaml` into one `.sshu` zip
  (0600 — it carries the same plaintext passwords the YAML files do; refuses
  to overwrite, appends the extension when left off). A page, not a popup:
  letters and digits type, `Tab` moves fields, `Enter` runs, `Esc` returns to
  the nav.
- **Import** — read a `.sshu` bundle and append its entries, keyed by name:
  a name you already have is skipped whole, invalid entries too, and the
  summary counts added and skipped — on the page, as a toast, and in the app
  log.
- **Splash byline** — the easter egg signs off with
  `developed by vulcan.shen.2304@gmail.com`.

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
