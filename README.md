# sshu

<p align="center"><img src="docs/icon.svg" width="128" alt="sshu icon" /></p>

[![GitHub Release](https://img.shields.io/github/v/release/vulcanshen/sshu)](https://github.com/vulcanshen/sshu/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/vulcanshen/sshu)](https://go.dev/)
[![License](https://img.shields.io/badge/license-GPL--3.0-blue)](LICENSE)
[![Charm in the Wild](https://img.shields.io/static/v1?label=Listed%20in&message=Charm%20in%20the%20Wild&color=6B5CE7)](https://github.com/charm-and-friends/charm-in-the-wild#networking-and-file-transfer)

**Language**: English · [繁體中文](README-zh_TW.md)

**A terminal front end for ssh and sftp** — `Tab` / `Enter` / `Esc` / `Space` / `?` drive everything. Keep your hosts in one file, open as many shells as you like, and move files between any two machines side by side. No hotkey memorization, no setup, no learning curve.

> _When in doubt, hit_ **`Space`**.

sshu is a member of the `u`-family and an ssh-domain implementation of [Vulcan's TUI Design Principle](https://github.com/vulcanshen/thoughts/blob/main/vtp.md) — the same design system as [kbu](https://github.com/vulcanshen/kbu) (Kubernetes) and [filu](https://github.com/vulcanshen/filu) (filesystem). See [`docs/sshu-implementation.md`](docs/sshu-implementation.md) for the clause-by-clause record, and [`docs/sshu-ui-design.md`](docs/sshu-ui-design.md) for the reasoning behind it — including the approaches that were tried and rejected.

The inspiration is [Termius](https://termius.com/) — a GUI SSH client, not another terminal tool. sshu borrows its spirit — hosts, sessions and transfers under one roof — not its feature list.

## Demo

### The manage tab — hosts, credentials, logs, and a connect
![manage](docs/demo-preference.gif)

### Two-sided file transfer — marks, a real transfer, the rule as its progress bar
![file transfer](docs/demo-transfer.gif)

### The ssh grid — cells, layouts, held-Alt arrows
![ssh grid](docs/demo-grid.gif)

## Five keys to drive sshu

| Key | Behavior |
|---|---|
| **`Tab`** | Move focus to the next panel of this tab (on the ssh tab it toggles a session's cell instead) |
| **`Enter`** | Connect / enter a directory / commit a choice |
| **`Space`** | *What can I do here?* — the contextual menu for whatever has focus. Also closes any popup |
| **`Esc`** | Back out — leave a search, go up a directory, close the top popup |
| **`?`** | Global help — every app-wide action in one list |

Tabs are switched with a single shifted letter — **`M` / `F` / `S`** — and every bare digit `1`–`9` addresses a panel of the current tab. Inside a pty the letters belong to the remote like every other bare key: `Alt+Esc` takes the keyboard back first.

When in doubt, press `Space`. Letter hotkeys exist for speed, and every one of them is also a row in the `Space` menu — so there is nothing you have to memorize unless you want to.

## The three tabs

```
 [Alt] ❯ [p]reference ❯ [f]ile transfer ❯ [s]sh
```

**`[M]anage`** — everything that is sshu's own, under one nav: **SSH** (Hosts, Credentials) and **Events** (Logs). Hosts are a table over `hosts.yaml`, one row each, shedding columns as the terminal narrows; `[A]dd` / `[E]dit` open a form with live validation, `Enter` connects. Credentials are reusable identities (user + auth) that hosts can reference with `auth: credential`. Logs are everything that happened while you were not looking, persisted to disk — and `[C]lear logs` empties them, `applogs.yaml` included.

**`[F]ile transfer`** — two independent filesystems side by side, 1:1. `local` opens where you launched sshu, so `cd ~/release && sshu` is already looking at the release. Either end can be this machine or a saved host, and both ends can be remote, so upload, download and remote-to-remote are one operation rather than three. Mark what you want, cross to the other side, and send it. While bytes move, the `<done>/<files> · <pct>%` summary in the top right reports in green, and the rule under the tab row doubles as a progress bar — green ink filling from the left with the percentage, on every tab, snapping back to a plain line when the transfer ends. `/` searches the **whole subtree**, not just the directory on screen; `v` reads a file without fetching it and `e` opens one in your own editor.

**`[S]SH`** — a **grid of live terminals**, each a real `ssh` on its own PTY. `Tab` on the sessions list toggles a session's cell on the grid, `Enter` shows one and hands it the keyboard, holding `Alt` the arrow keys steer between cells, `Alt+Esc` takes the keyboard back. As the cursor walks the sessions list, the matching cell lights up on the grid. A layout strip arranges the grid: horizontal, vertical, or a custom rows × columns.

## Install

> sshu is **macOS / Linux only** — it uses a Unix PTY. No native Windows build.

**Homebrew** (macOS / Linux):

```bash
brew install vulcanshen/tap/sshu
```

**Install script** (drops the latest release binary into `~/.local/bin`, or `/usr/local/bin` as root):

```bash
curl -fsSL https://raw.githubusercontent.com/vulcanshen/sshu/main/install.sh | sh
```

**From source**:

```bash
go install github.com/vulcanshen/sshu/cmd/sshu@latest
```

or clone and build:

```bash
git clone https://github.com/vulcanshen/sshu.git
cd sshu
make build     # → ./sshu   (CGO_ENABLED=0, -trimpath, stripped)
./sshu
```

A `Makefile` wraps the common tasks — `make build`, `make install` (→ `$GOBIN`) / `make uninstall`, `make demo` (runs against `demo/hosts.yaml` without touching your real config), `make package` (a `.tar.gz` under `dist/`), and `make check` (fmt + vet + test). Run `make` to list them.

**A Nerd Font is required**, not optional: auth methods, file types and marks are drawn with Nerd Font glyphs, and the layout measures them.

### Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/vulcanshen/sshu/main/uninstall.sh | sh
```

Removes the binary, then asks — never assumes — about the config directory, because `hosts.yaml` and `credentials.yaml` live there.

## Quick start

```bash
sshu
```

Opens on the hosts table. Press `[A]` to add your first host, `Enter` to connect, `F` for the file browser. If you have never used sshu before, press `Space` on any panel and read the menu — it lists exactly what that panel can do.

## Where your data lives

A few YAML files in one directory, resolved in this order:

| | |
|---|---|
| `$SSHU_CONFIG` | names the directory outright (used by `make demo` and by tests) |
| `$XDG_CONFIG_HOME/sshu` | when set — on macOS too, so you can opt out of `~/Library/Application Support` |
| otherwise | `os.UserConfigDir()/sshu` |

`hosts.yaml` holds the hosts; `credentials.yaml` holds reusable identities — a name, a user and how that user authenticates — which a host can take wholesale with `auth: credential` + `credential: <name>`, so "who does this connection run as" is written in exactly one place. `applogs.yaml` is the app log's spine on disk. All three are hand-editable YAML, and every write is atomic (temp file + rename) and re-asserts mode `0600`.

### Settings — `config.yaml`

Optional, in the same directory, and **sshu never writes it**: a file you have edited is never reformatted and your comments survive. A missing file means the defaults, so it only has to say what you want changed.

```yaml
# How long one connection attempt gets, in seconds. Default 15.
# The ssh tab hands it to ssh as -o ConnectTimeout; the sftp side uses it to bound its dial.
connect_timeout: 15
```

A value outside 1–600 is treated as a slipped decimal and the default is used instead. A file that cannot be parsed does not stop sshu from starting — it runs on the defaults and says so in the app log.

### Passwords are stored in plaintext — read this

A host with `auth: password` keeps its password in `hosts.yaml` **in the clear**, and a credential with `auth: password` does the same in `credentials.yaml`. That is a deliberate trade, and these are the mitigations:

- the file is kept at `0600`, re-asserted on every write, and carries a warning header
- the password is never rendered — the form shows `••••`
- `SSH_ASKPASS` supplies it to `ssh`, so the secret **never enters a child process's environment** and never appears in `ps`

`0600` does not survive being copied into a backup, a dotfiles repo, or a synced folder. If that matters to you, use `auth: privatekey`, which stores only a path. A keychain-backed `secretStore` is a planned alternative.

### Host keys

The ssh tab shells out to the real `ssh` binary, so host-key handling there is OpenSSH's, with your `~/.ssh/config` and `known_hosts`.

The file-transfer tab speaks the protocol itself, and its policy is stricter: an **unknown** host is refused rather than waved through, and a **changed** key is refused outright and never offered as a question. To accept a new host, connect to it once through the ssh tab — that is OpenSSH's prompt, with OpenSSH's fingerprint.

## Key bindings

Every letter hotkey below is also a row in that panel's `Space` menu. The bracket shows the key **exactly as you press it**: `[A]dd` is shift+A, `[t]ransfer` is a bare `t`, and nothing fires that the marking does not name.

### Everywhere

```
 tabs      M / F / S                 (inside a pty: they are the remote's)
 panels    1–9 of the current tab  ·  Tab (ssh tab: display toggle)
 cursor    j k    u d (half page)     gg G      arrows are synonyms
 global    Space menu    ? help    q quit    Ctrl+C force quit
           (in a pty or an editor, Ctrl+C is theirs — Alt+Esc first)
```

### `[M]anage`

The left nav (`1`) picks a section — **Hosts**, **Credentials**, **Logs**, grouped under SSH / Events headers the cursor skips over — and the content follows the cursor; `Enter` or `2` moves the keyboard to the content. Hand the keyboard over and the whole nav dims to a legend for what `[2]` is showing; the only thing that stays lit is the unread-error count.

| Key | Action |
|---|---|
| `Enter` | hosts: Connect (asks first; a credential host is resolved right here) · credentials: Edit |
| `A` | Add a host / a credential |
| `E` | Edit the host under the cursor |
| `D` | Delete it (asks first — deleting a credential counts the hosts that still reference it) |
| `/` | hosts: Search — name, user, host and port at once, ranked best-first |
| `C` | logs: Clear the log (asks first — it erases `applogs.yaml` too) |

In the forms: `Tab` / `Shift+Tab` / `↑` `↓` move between fields; `←` `→` switch Auth (password / privatekey / **credential**). On the two pick-a-value fields — IdentityFile and Credential — **`Enter` on the empty field opens the chooser, `Enter` on a filled one saves like it does anywhere else on the form, and `Backspace` clears the whole line**. Choosing `credential` darkens the User row: the credential supplies the user.

### `[F]ile transfer` — lower case is the row, upper case is the panel

| Key | Action |
|---|---|
| `h` `l` | Cross to the other half, keeping the row (`[2]`↔`[4]`) |
| `Enter` | Enter the directory under the cursor — or go to whatever the search found |
| `a` | **Append to marks** — press it again to take the mark off. Refused on a file that is still arriving: a mark says the path is a thing you can act on, and half a file is not |
| `r` | Rename it, in place |
| `v` | **View it** — text with syntax highlighting and line numbers, a binary as hex, a directory as its listing |
| `e` | **Edit it** in `$EDITOR` — fetched, edited, written back |
| `t` | Transfer it to the other side's current directory |
| `x` | Delete it (asks first) |
| `/` | **Search the whole subtree** — `Enter` goes to a result and leaves the cursor on it, where `a` / `t` / `v` / `e` / `x` all work |
| `A` | **Add** here — `name` makes an empty file, `name/` makes a directory |
| `R` | **Refresh** — re-read this directory now. The background poll only re-lists when the directory's timestamp moved, and a timestamp is not a promise |
| `T` | Transfer every mark on this side |
| `X` | Delete every mark on this side (asks first) |
| `c` / `C` | Clear one mark (on a marks panel) / clear them all — forgets them, changes nothing on disk |
| `H` | **Host** — switch this side. `local` is first, and it opens **the directory you launched sshu in**. On a side that has no host yet, `Space` opens this list directly: a menu of one row is not an answer |
| `D` | **Disconnect** — this side goes back to having no host at all |
| `J` | **Jobs** — transfers in flight, with per-job cancel |

A file being written into shows a spinner **in its mark column** — it exists but
is not all there yet — and so does the directory it is landing in, since that is
the row you can actually see when a whole tree is being copied. Both clear and
the listing re-reads the moment the job ends.

While bytes are moving, `S` and `D` are **frozen**: both swap the filesystem out
from under a side, and every transfer has both sides on it. The two rows stay in
the Space menu and dim rather than disappearing — they belong on this panel, they
are just unavailable this second — and pressing either says to cancel in `P`
first. The top-right summary spins while anything is in flight.

### `[S]SH`

| Key | Action |
|---|---|
| **`Tab`** | **Toggle this session's cell** on the grid — any number can be up at once |
| `Enter` | Show this session **and hand it the keyboard** (the side column folds away) |
| `C` | Close this session (asks first) |
| *(no key — `Space` menu only)* | **Close all sessions** — ends every one of them, asking first with the count in the question. Deliberately without a letter: closing everything is destructive and rare, and a letter is what a hand finds by accident on a list it was only scrolling |
| `D` | Duplicate — a second session to the same host (asks first). The keyboard **stays on the list**, with the cursor on the new session: the Enter you pressed was on a confirmation, and only an Enter on a row means "take me in" |
| **`PgUp` / `PgDown`** | **Page through this cell's history** — while the remote is not in the alt screen; anything you type snaps back to live |
| **`Alt+Enter`** | **Zoom** — this cell fills the whole grid. With only one cell there is nothing to zoom, so the chord goes to the remote instead |
| **`Alt+arrows`** | Steer to the neighbouring cell — spatial, so nothing has to be numbered. Inside a zoom it still steers, and stays zoomed |
| **`Alt+Esc`** | **Out, one layer at a time** — the first press leaves a zoom, the next takes the keyboard back from the remote (back to the list, side column returns) |

The layout strip (`2`, bottom of the left column — the right side is nothing but terminals): `j`/`k` walk **horizontal / vertical / custom** and apply as you move; `Enter` on custom asks for rows × columns (any two digits 1–9). Rows read `<user>@<host>:<port>` on one line, ssh's own spelling, and lead with a display column — a monitor glyph for a session with a cell on the grid, a struck-through one without. As the cursor moves, the matching cell's border lights on the grid — the row and its terminal are the same session, so they light together. That light is the **cursor's** colour, not the focus blue: blue means the keyboard is here, and two blue frames on screen would make you hunt for which one is live.

`Alt+Esc` is sshu's own key and exists for exactly one situation: a grid cell hands every keystroke to the remote, so something has to be able to take it back. Everywhere else, plain `Esc` is enough. `Alt+Enter` is its opposite number — `Enter` is "go in" all over sshu, and a zoom is going further in — which is why `Alt+Esc` is what comes back out of one.

`PgUp` / `PgDown` are borrowed rather than taken: a full-screen program pages with them itself and announces itself by switching to the alt screen, so while one is up the keys go straight through to it. Plain shell output pages nothing, which is when scrolling has to come from somewhere. A cell showing history says so in its title (`󰋚` and how far back), because a cell showing the past and a cell whose remote has gone quiet are otherwise the same still picture.

## Features

- **Zero learning curve** — every action surfaces through the `Space` menu, in context, on every panel. The menu and the letter hotkey are generated from one table, so a hotkey that is not in the menu cannot exist.
- **Menus in two regions** — `item` (what happens to the row under the cursor, named by that row) and `panel` (what happens to this side). A menu with only one region stays flat.
- **A grid of concurrent ssh sessions** — each a real `ssh` in an embedded PTY, any number on screen at once, arranged horizontally, vertically or in a custom rows × columns. Each cell's remote is told its own size, and only when it actually changes. Ended sessions leave the grid and release their emulator immediately; the keyboard never silently lands in another remote.
- **Reusable credentials** — a user plus how that user authenticates, saved once in `credentials.yaml` and referenced by any number of hosts with `auth: credential`. Resolution happens at the doors: the connect confirmation shows who the session will actually run as, and a dangling reference fails there with a sentence, not inside ssh.
- **Two-sided sftp** — local ↔ remote ↔ remote through one `FS` interface. Marks are per side; a mark is an absolute path, so it follows a rename and is dropped when the file is deleted.
- **Recursive subtree search** — `/` walks the whole tree beneath the current directory, **breadth-first** (over SFTP each directory is a round trip, so what is near arrives first), streaming, cancellable, capped, and drawn **in place**. `Enter` takes you to a result with the cursor already on it, so from there marking and transferring it needs nothing new.
- **Read before you fetch** — `v` shows the item under the cursor: text syntax-highlighted with line numbers (chroma, catppuccin-mocha — the same as filu), a binary as an xxd-style hex dump, a directory as one level of its listing. It reads at most 64 KiB, because on a remote side every byte of that crosses the network. Escape sequences in the file are stripped: those bytes come off someone else's machine and would otherwise repaint your terminal.
- **Edit in your own editor** — `e` opens the item under the cursor in `$VISUAL` / `$EDITOR` (`vi` only as a floor, never a dependency), running inside the embedded terminal so the frame stays up. A remote file is fetched, edited and written back; a local one is edited where it lives, so its inode — and every hard link to it — survives. Nothing is written back unless the content actually changed, the write lands atomically so a dropped link cannot leave a truncated config behind, and a file that somebody else changed while you had it open is never overwritten without asking.
- **A real transfer engine** — the whole plan is computed before anything is written, so the progress bar's denominator is right from the first frame and overwrites are asked about once, up front. Per-job cancel; a cancelled or failed file is removed rather than left looking complete.
- **Directories that stay current, cheaply** — SFTP has no change notification, so sshu stats the directory and compares its mtime, and re-lists only when that moves. One small round trip every couple of seconds instead of a full listing, and only while the tab is on screen.
- **Terminal history that vt10x does not keep** — the emulator is a fixed grid and clears the rows that leave the top, so every chunk read from the PTY is split into lines and filed as it goes in, colours and all. `PgUp` / `PgDown` page through the last 10 000 lines. Nothing is captured while the alt screen is up: a full-screen program repaints its whole window on every keystroke, and capturing that would flush the shell history the buffer exists to hold. `\x1b[3J` — a remote explicitly erasing its scrollback — drops it, `\x1b[2J` does not, so `clear` behaves here exactly as it does in your own terminal.
- **A connection that has not answered yet says so** — a grid cell draws the PTY, and ssh prints nothing at all while it waits for TCP, so an unreachable host used to leave an empty box for as long as the OS took to give up. The test is whether the far end has sent a byte, not whether the grid is empty: until it does, the panel names the host and counts the seconds.
- **Nothing dies silently, and nothing is only said once** — a session that ends badly raises a toast naming the host and **what ssh itself said** (`Connection refused`, not `disconnected`), the grid keeps saying it instead of going blank, and the app log holds **the whole final screen** — a refused connection is one line, but a host key mismatch is fifteen and the fingerprint you need is in the middle of them. The log lives at manage → logs, **persisted to `applogs.yaml`** so it survives the process, and it records more than failures: hosts and credentials changing, connections opening and closing, transfers ending, edits written back. The nav and the footer count the errors you have not read until you look.
- **No exit leaves an orphan** — every child ssh runs on its own PTY session, where no signal would reach it on its own. A registry knows them all, and every way out — `q`, `Ctrl+C`, an outside SIGINT/SIGTERM, even the terminal window closing (SIGHUP) — kills them on the way.
- **Frame stability** — every rendered line is exactly the terminal width, at every size, with any content. Wide characters from a remote, Nerd Font glyphs that measure differently, and CJK filenames are all handled by measuring rather than assuming; there is a test that checks it across sizes, focus states and data.
- **unix-first, static binary** — macOS + Linux; `CGO_ENABLED=0`.

## Status

**v1.2.0.** Three tabs on one shifted letter each, the grouped `[1] sshu` nav, reusable credentials, the persistent app log, the ssh terminal grid with pageable history, and no exit that leaves an orphan. 300+ tests, `make check` green and `-race` clean. See [CHANGELOG.md](CHANGELOG.md).

Not there yet:
- **interactive host-key confirmation for the sftp side** — today an unknown host is refused and you accept it through the ssh tab
- **encrypted private keys** for the sftp side — reported plainly, but not usable; agent support is the likely answer
- content search on a remote (it would mean running a command on the far end, which this tab deliberately does not do)
- an `[S]ftp` shortcut on the hosts table, to send the host under the cursor straight to the focused side of the file browser
- mouse support, `fsnotify` reload of `hosts.yaml`, session persistence, keychain-backed password storage

## Built with

Go, [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss), [creack/pty](https://github.com/creack/pty) + [hinshun/vt10x](https://github.com/hinshun/vt10x) for the embedded terminals, [pkg/sftp](https://github.com/pkg/sftp) + `golang.org/x/crypto/ssh` for the file transfers, and [chroma](https://github.com/alecthomas/chroma) for syntax highlighting in `v`. Colours are catppuccin-mocha.
