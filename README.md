# sshu

**A terminal front end for ssh and sftp** — `Tab` / `Enter` / `Esc` / `Space` / `?` drive everything. Keep your hosts in one file, open as many shells as you like, and move files between any two machines side by side. No hotkey memorization, no setup, no learning curve.

> _When in doubt, hit_ **`Space`**.

sshu is a member of the `u`-family and an ssh-domain implementation of [Vulcan's TUI Design Principle](https://github.com/vulcanshen/thoughts/blob/main/vtp.md) — the same design system as [kbu](https://github.com/vulcanshen/kbu) (Kubernetes) and [filu](https://github.com/vulcanshen/filu) (filesystem). See [`docs/sshu-implementation.md`](docs/sshu-implementation.md) for the clause-by-clause record, and [`docs/sshu-ui-design.md`](docs/sshu-ui-design.md) for the reasoning behind it — including the approaches that were tried and rejected.

## Five keys to drive sshu

| Key | Behavior |
|---|---|
| **`Tab`** | Move focus to the next panel of this tab (`1`–`3` switch tab, `4`–`7` jump to a panel) |
| **`Enter`** | Connect / enter a directory / commit a choice |
| **`Space`** | *What can I do here?* — the contextual menu for whatever has focus. Also closes any popup |
| **`Esc`** | Back out — leave a search, go up a directory, close the top popup |
| **`?`** | Global help — every app-wide action in one list |

When in doubt, press `Space`. Letter hotkeys exist for speed, and every one of them is also a row in the `Space` menu — so there is nothing you have to memorize unless you want to.

## The three tabs

```
 ([1] hosts)  ([2] sftp)  ([3] ssh)
```

**`[1]` hosts** — a table over `hosts.yaml`: name, user, host, port and auth method, one row each, shedding columns as the terminal narrows. `[A]dd` / `[E]dit` open a form with live validation; `Tab` on the IdentityFile field opens a fuzzy file picker that shows permissions and flags a key other people can read. `Enter` connects.

**`[2]` sftp** — two independent filesystems side by side, 1:1. Either end can be this machine or a saved host, and both ends can be remote, so upload, download and remote-to-remote are one operation rather than three. Mark what you want, cross to the other side, and send it. `/` searches the **whole subtree**, not just the directory on screen.

**`[3]` ssh** — many concurrent sessions, each a real `ssh` in an embedded terminal, one shown at a time. `[5]` takes the whole tab while the remote has the keyboard; `Alt+Esc` takes it back.

## Install

> sshu is **macOS / Linux only** — it uses a Unix PTY. No native Windows build.

There is no release binary yet. Build from source:

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

## Quick start

```bash
sshu
```

Opens on `[1]` hosts. Press `[A]` to add your first host, `Enter` to connect, `2` for the file browser. If you have never used sshu before, press `Space` on any panel and read the menu — it lists exactly what that panel can do.

## Where your data lives

One file, `hosts.yaml`, resolved in this order:

| | |
|---|---|
| `$SSHU_CONFIG` | names the directory outright (used by `make demo` and by tests) |
| `$XDG_CONFIG_HOME/sshu` | when set — on macOS too, so you can opt out of `~/Library/Application Support` |
| otherwise | `os.UserConfigDir()/sshu` |

It is hand-editable YAML and sshu says so in the file's own header. Writes are atomic (temp file + rename) and re-assert mode `0600` every time.

### Passwords are stored in plaintext — read this

A host with `auth: password` keeps its password in `hosts.yaml` **in the clear**. That is a deliberate trade, and these are the mitigations:

- the file is kept at `0600`, re-asserted on every write, and carries a warning header
- the password is never rendered — the form shows `••••`
- `SSH_ASKPASS` supplies it to `ssh`, so the secret **never enters a child process's environment** and never appears in `ps`

`0600` does not survive being copied into a backup, a dotfiles repo, or a synced folder. If that matters to you, use `auth: privatekey`, which stores only a path. A keychain-backed `secretStore` is a planned alternative.

### Host keys

The `[3]` ssh tab shells out to the real `ssh` binary, so host-key handling there is OpenSSH's, with your `~/.ssh/config` and `known_hosts`.

The `[2]` sftp tab speaks the protocol itself, and its policy is stricter: an **unknown** host is refused rather than waved through, and a **changed** key is refused outright and never offered as a question. To accept a new host, connect to it once through `[3]` — that is OpenSSH's prompt, with OpenSSH's fingerprint.

## Key bindings

Every letter hotkey below is also a row in that panel's `Space` menu. The bracket shows the key **exactly as you press it**: `[A]dd` is shift+A, `[t]ransfer` is a bare `t`, and nothing fires that the marking does not name.

### Everywhere

```
 tabs      1 2 3                      panels    4 5 6 7  ·  Tab (this tab only)
 cursor    j k    u d (half page)     gg G      arrows are synonyms
 global    Space menu    ? help    q quit    Ctrl+C force quit
```

### `[1]` hosts

| Key | Action |
|---|---|
| `Enter` | Connect (asks first) |
| `A` | Add a host |
| `E` | Edit the host under the cursor |
| `D` | Delete it (asks first) |
| `/` | Search — name, user, host and port at once (**not** the auth column), ranked best-first |

In the form: `Tab` / `Shift+Tab` / `↑` `↓` move between fields, `←` `→` switch the Auth field, **`Tab` on IdentityFile opens the file picker**, `Enter` submits, `Esc` cancels.

### `[2]` sftp — lower case is the row, upper case is the panel

| Key | Action |
|---|---|
| `h` `l` | Cross to the other half, keeping the row (`[5]`↔`[7]`) |
| `Enter` | Enter the directory under the cursor |
| `m` | Mark / unmark it (on a marks panel, `m` unmarks) |
| `r` | Rename it, in place |
| `v` | **View it** — text with syntax highlighting and line numbers, a binary as hex, a directory as its listing |
| `t` | Transfer it to the other side's current directory |
| `x` | Delete it (asks first) |
| `/` | **Search the whole subtree** — results are ordinary rows, so `m` / `t` / `x` work on them |
| `N` | New directory here |
| `T` | Transfer every mark on this side |
| `X` | Delete every mark on this side (asks first) |
| `C` | Clear the marks — forgets them, changes nothing on disk |
| `S` | Select host (`local` is always the first choice) |
| `P` | Progress — running transfers, with per-job cancel |

### `[3]` ssh

| Key | Action |
|---|---|
| `Enter` | Show this session in `[5]` (no confirmation — switching costs nothing) |
| `l` | Cross into `[5]` |
| `C` | Close this session (asks first) |
| `D` | Duplicate — a second session to the same host (asks first) |
| `H` | History — sessions that have ended, and why |
| **`Alt+Esc`** | **Take the keyboard back from the remote** |

Rows read `<user>@<host>` with the port at the right edge — what the connection is, not what it is called — and the one `[5]` is showing is green.

`Alt+Esc` is sshu's own key and exists for exactly one situation: `[5]` hands every keystroke to the remote, so something has to be able to take it back. Everywhere else, plain `Esc` is enough.

## Features

- **Zero learning curve** — every action surfaces through the `Space` menu, in context, on every panel. The menu and the letter hotkey are generated from one table, so a hotkey that is not in the menu cannot exist.
- **Menus in two regions** — `item` (what happens to the row under the cursor, named by that row) and `panel` (what happens to this side). A menu with only one region stays flat.
- **Many concurrent ssh sessions** — each a real `ssh` in an embedded PTY. `[5]` takes the whole tab while focused; ended sessions release their terminal emulator immediately rather than freezing on a dead screen.
- **Two-sided sftp** — local ↔ remote ↔ remote through one `FS` interface. Marks are per side; a mark is an absolute path, so it follows a rename and is dropped when the file is deleted.
- **Recursive subtree search** — `/` walks the whole tree beneath the current directory, **breadth-first** (over SFTP each directory is a round trip, so what is near arrives first), streaming, cancellable, capped, and drawn **in place**: a result is an ordinary row, so marking and transferring it needs nothing new.
- **Read before you fetch** — `v` shows the item under the cursor: text syntax-highlighted with line numbers (chroma, catppuccin-mocha — the same as filu), a binary as an xxd-style hex dump, a directory as one level of its listing. It reads at most 64 KiB, because on a remote side every byte of that crosses the network. Escape sequences in the file are stripped: those bytes come off someone else's machine and would otherwise repaint your terminal.
- **A real transfer engine** — the whole plan is computed before anything is written, so the progress bar's denominator is right from the first frame and overwrites are asked about once, up front. Per-job cancel; a cancelled or failed file is removed rather than left looking complete.
- **Directories that stay current, cheaply** — SFTP has no change notification, so sshu stats the directory and compares its mtime, and re-lists only when that moves. One small round trip every couple of seconds instead of a full listing, and only while the tab is on screen.
- **Nothing dies silently** — a session that ends badly raises a toast naming the host and the reason; a clean `exit` says nothing, because that is what you asked for.
- **Frame stability** — every rendered line is exactly the terminal width, at every size, with any content. Wide characters from a remote, Nerd Font glyphs that measure differently, and CJK filenames are all handled by measuring rather than assuming; there is a test that checks it across sizes, focus states and data.
- **unix-first, static binary** — macOS + Linux; `CGO_ENABLED=0`.

## Status

Working end to end: all three tabs, 190 tests, `make check` green.

Not there yet:

- no release binary, Homebrew tap or install script
- **interactive host-key confirmation for `[2]`** — today an unknown host is refused and you accept it through `[3]`
- **encrypted private keys** for `[2]` — reported plainly, but not usable; agent support is the likely answer
- content search on a remote (it would mean running a command on the far end, which this tab deliberately does not do)
- mouse support, `fsnotify` reload of `hosts.yaml`, session persistence, keychain-backed password storage

## Built with

Go, [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss), [creack/pty](https://github.com/creack/pty) + [hinshun/vt10x](https://github.com/hinshun/vt10x) for the embedded terminals, and [pkg/sftp](https://github.com/pkg/sftp) + `golang.org/x/crypto/ssh` for the file transfers. Colours are catppuccin-mocha.
