# sshu

<p align="center"><img src="docs/icon.svg" width="128" alt="sshu icon" /></p>

[![GitHub Release](https://img.shields.io/github/v/release/vulcanshen/sshu)](https://github.com/vulcanshen/sshu/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/vulcanshen/sshu)](https://go.dev/)
[![License](https://img.shields.io/badge/license-GPL--3.0-blue)](LICENSE)
[![Charm in the Wild](https://img.shields.io/static/v1?label=Listed%20in&message=Charm%20in%20the%20Wild&color=6B5CE7)](https://github.com/charm-and-friends/charm-in-the-wild#networking-and-file-transfer)

**Language**: English · [繁體中文](README-zh_TW.md)

**A terminal front end for ssh and sftp** — `Tab` / `Enter` / `Esc` / `Space` / `?` drive everything. Keep your hosts in one file, open as many shells as you like, and move files between any two machines side by side. Install it on the server too and it nests — sshu inside sshu, at any depth, with no layer costing you a row of screen. No hotkey memorization, no setup, no learning curve.

> _When in doubt, hit_ **`Space`**.

Inspired by [Termius](https://termius.com/) — hosts, sessions and transfers under one roof — but in your terminal.

## Demo

![ssh grid](docs/demo-grid.gif)

The ssh grid: many live sessions on one screen, each cell a real `ssh`.

## Highlights

- **Nothing to memorize** — `Space` on any panel lists everything that panel can do. Every letter hotkey is also a row in that menu.
- **A grid of live ssh sessions** — each cell is a real `ssh` on its own terminal, as many as you like on screen, arranged horizontally, vertically or in columns you choose. Page back through a session's history with `PgUp/PgDn`.
- **Copy out of a session with the keyboard** — `Alt-v` freezes a cell and lets you select with vim keys; `y` puts the text on the system clipboard.
- **File transfer between any two machines** — local ↔ remote ↔ remote in one view. Search a whole subtree, read a file before fetching it, or open it in your own `$EDITOR` and have it written back.
- **sshu inside sshu, at any depth** — install it on the server too. `Alt-z` takes a cell to full screen with no chrome, and `Alt-Enter` passes the keyboard down the chain, so nested layers cost nothing.
- **Your `~/.ssh` files, in place** — browse and edit `~/.ssh/config` and `~/.ssh/known_hosts` without losing a comment. An `auth: sshconfig` host stores nothing at all and lets `~/.ssh/config` decide — ProxyJump, agent and key included.
- **Reusable credentials** — define a user and how it authenticates once, and point any number of hosts at it.
- **Tags** — your own words on a host, searchable with `/`: type `prod` and the group is there.
- **Failures you can read** — a session that ends badly says what ssh itself said, and the full output is kept under manage → Errors, alongside a record of every connection and every change you made.
- **Passwords encrypted on disk** — and never shown, never passed through the environment.

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

Building from source is in [`docs/dev-remarks.md`](docs/dev-remarks.md).

### Requirements

- **A Nerd Font** — not optional: auth methods, file types and marks are drawn with Nerd Font glyphs. Fonts made for CJK that draw icons two cells wide (e.g. Maple Mono NF CN) work too: sshu asks the terminal how wide an icon is when it starts, and a sshu nested inside it is told the same. If it guesses wrong, set `SSHU__ICON_WIDTH=2` (or `1`).
- **A truecolor terminal** (24-bit colour) — the theme's softer shades, the popup layers and the dimming behind a popup do not survive 256 colours.
- **A terminal that sends Alt** — sshu's `Alt-…` keys (`Alt-Esc` to leave a session, `Alt-v`, `Alt-z`, …) need the Option key to act as Meta. Turn on *Use Option as Meta key* in macOS Terminal, or set Option to *Esc+* in iTerm2; kitty, Alacritty and WezTerm send it by default.

### Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/vulcanshen/sshu/main/uninstall.sh | sh
```

Removes the binary, then asks before touching the config directory, since your hosts and credentials live there.

## Quick start

```bash
sshu
```

It opens on the hosts table. Press `A` to add your first host, `Enter` to connect, `F` for file transfer. If you are not sure what to do next, press `Space`.

## Five keys to drive sshu

| Key | Behavior |
|---|---|
| **`Tab`** | Move focus to the next panel of this tab (manage and file transfer; on the ssh tab use `1–2`) |
| **`Enter`** | Connect / enter a directory / commit a choice |
| **`Space`** | *What can I do here?* — the menu for whatever has focus; its last row opens the global operations (switch tab, quit). Press it again to close the menu |
| **`Esc`** | Back out — leave a search, go up a directory, close the top popup |
| **`?`** | The keys of the panel you are on, then the ones that work everywhere. On a popup: that popup's own keys |

Switch tabs with **`M/F/S`**; the digits `1`–`9` jump to a panel of the current tab. While you are typing into a remote session every key belongs to the remote — press `Alt-Esc` to take the keyboard back. It asks first, because a quick double `Esc` (everyday in vim) can arrive as `Alt-Esc`: `Enter` leaves, `Esc` goes back in.

## The three tabs

```
 [M]anage ❯ [F]ile transfer ❯ [S]SH
```

**`[M]anage`** — everything sshu knows, in three groups:

- **SSHU** — **Hosts** (a searchable table over `hosts.yaml`; `A` adds, `E` edits, `Enter` connects) and **Credentials** (reusable identities hosts can reference).
- **SSH** — **Config** (`~/.ssh/config`, one `Host` block per row, `Include` followed; editing a block touches only its own lines) and **KnownHosts** (`~/.ssh/known_hosts`; fetch a host's key and see its fingerprint before trusting it, or remove a stale one).
- **Logs** — **Errors** (what went wrong; `Enter` shows everything the far end printed), **Connections** (every ssh and sftp attempt) and **Changes** (what you edited); on these two `Enter` opens the whole record, every entry in full. Each can be cleared on its own.

**`[F]ile transfer`** — two sides, each either this machine or a saved host, so upload, download and remote-to-remote are the same operation. Mark files, cross to the other side, send. `local` opens in the directory you launched sshu from, so `cd ~/release && sshu` is already looking at the release. Progress shows in the top-right corner and as a bar under the tab row.

**`[S]SH`** — a grid of live terminals. `Enter` on a session hands it the keyboard, `Alt-←/→/↑/↓` move between cells, `Alt-z` makes the focused cell bigger, `Alt-Esc` takes the keyboard back (asking first). A layout strip picks horizontal, vertical or a number of columns.

## Key bindings

Every letter hotkey below is also a row in that panel's `Space` menu. The bracket shows the key **exactly as you press it**: `[A]dd` is `Shift-A`, `[t]ransfer` is a bare `t`.

### Everywhere

```
 tabs      M/F/S                     (inside a session: they are the remote's)
 panels    1–9 of the current tab  ·  Tab (manage, file transfer)
 cursor    j/k    u/d (half page)    gg/G      arrows are synonyms
 global    Space menu    ? help    q quit    Ctrl-C quit (twice: at once)
           (in a session or an editor, Ctrl-C is theirs — Alt-Esc first)
```

### `[M]anage`

The left nav (`1`) picks a section and the content follows the cursor; `Enter` or `2` moves the keyboard to the content.

| Key | Action |
|---|---|
| `Enter` | Show the row in full, read-only — passwords are always masked. On a host, the foot offers to connect: `Enter` again goes in. On Errors: the full output |
| `A` | Add a host / credential / `~/.ssh/config` block / known_hosts key (fetched from the host, then you decide) |
| `E` | Edit the row under the cursor |
| `D` | Duplicate a host, credential or `~/.ssh/config` block — an Add form pre-filled from this row; give it a new name and save |
| `X` | Delete (asks first) |
| `/` | Search hosts — name, user, host, port and tags at once |
| `C` | Errors / Connections / Changes: clear this record (asks first) |

In the host form: `Tab/Shift-Tab/↑/↓` move between fields, `←/→` switch Auth between **password**, **privatekey**, **credential** and **sshconfig**. `Enter` saves; if something required is missing or wrong, it takes you to the first such field and says what is wrong. **Tags** are optional and separated by spaces.

### `[F]ile transfer` — lower case acts on the row, upper case on the panel

| Key | Action |
|---|---|
| `h/l` | Cross to the other side |
| `Enter` | Enter the directory — or go to a search result |
| `a` | Mark / unmark |
| `r` | Rename |
| `v` | View — text with syntax highlighting, binary as hex, a directory as its listing |
| `e` | Edit in `$EDITOR` — remote files are fetched and written back. In the editor `Alt-Esc` abandons the edit, asking first |
| `t` | Transfer to the other side's current directory |
| `x` | Delete (asks first) |
| `/` | Search the whole subtree |
| `A` | Add — `name` makes a file, `name/` a directory |
| `R` | Refresh this directory |
| `T` | Transfer every mark on this side |
| `X` | Delete every mark on this side (asks first) |
| `c/C` | Clear one mark / all marks (nothing on disk changes) |
| `H` | Pick the host for this side (`local` is first) |
| `D` | Disconnect this side |
| `J` | Jobs — transfers in flight; `Enter` opens one in full, `c` cancels it |

A row that cannot run right now is dimmed, in the `Space` menu and in `?`, and its key does nothing: `H` and `D` while a transfer is running (cancel it in `J` first), `a` on a file still arriving, `e` on a directory, `t` and `T` while the other side has no host. With nothing marked, `T`, `X` and `C` are not offered.

### `[S]SH`

| Key | Action |
|---|---|
| `H` | Hide this session's cell from the grid, or show it again |
| `Enter` | Show this session and hand it the keyboard |
| `C` | Close this session (asks first) — *Close all sessions* is in the `Space` menu |
| `D` | Open a second session to the same host (asks first) |
| `PgUp/PgDn` | Page through this cell's history (typing snaps back to live) |
| `Alt-z` | Bigger, in stages: fill the grid, then the whole screen, then back |
| `Alt-Enter` | Lock / release — for a nested sshu: pass every key through to the one inside |
| `Alt-←/→/↑/↓` | Move to the neighbouring cell |
| `Alt-Esc` | Back out one step — selection mode, then zoom, then the keyboard (that last step asks first) |
| `Alt-v` | Selection mode — freeze this cell to copy from it; its frame turns yellow and says `Select` |

**Selection mode** (`Alt-v`) uses vim's keys:

| Key | Action |
|---|---|
| `h/j/k/l` | Move (past the top or bottom, the frozen page scrolls) |
| `w/e/b` | Next word start / word end / previous word start |
| `0/$` | Start / end of the line |
| `u/d` | Half a screen up / down |
| `gg/G` | The first / last line, scrollback included |
| `v/V` | Select by character / by line |
| `y` | Copy to the system clipboard and leave (nothing selected: the current line) |
| `Esc` | Drop the selection, then leave |
| `?` | Every key the mode has |

Copying uses `pbcopy`, `wl-copy`, `xclip` or `xsel`, whichever is installed.

The layout strip (`2`, bottom left): `j/k` switch between **horizontal**, **vertical** and **custom**; `Enter` on custom asks for a number of columns.

## Configuration

### Where your data lives

All of sshu's files live in one directory:

| | |
|---|---|
| `$SSHU__CONFIG` | if set, this directory |
| `$XDG_CONFIG_HOME/sshu` | if set — on macOS too |
| otherwise | `os.UserConfigDir()/sshu` (`~/Library/Application Support/sshu` on macOS, `~/.config/sshu` on Linux) |

| File | Holds |
|---|---|
| `hosts.yaml` | your hosts |
| `credentials.yaml` | reusable identities — a name, a user and how it authenticates; a host uses one with `auth: credential` + `credential: <name>` |
| `config.yaml` | settings (optional, see below) |
| `errors.yaml` · `connections.yaml` · `changes.yaml` | the three logs |
| `.sshukey` | the key that encrypts stored passwords |

Everything is plain YAML you can edit by hand, kept at mode `0600`.

### Settings — `config.yaml`

Optional; sshu only reads it, so your comments and formatting are safe. Anything left out uses the default.

```yaml
# How long one connection attempt gets, in seconds (1–600). Default 15.
connect_timeout: 15
```

### Passwords

Stored passwords are encrypted with AES-256-GCM, so a `password:` field reads `ENC:…`. The key is `.sshukey` in the same directory, created on first run; `SSHU__KEY_FILE` moves it elsewhere.

|  |  |
|---|---|
| **protects** | `hosts.yaml` or `credentials.yaml` leaking on its own — pasted into a chat, committed by accident, picked up by a backup |
| **does not protect** | the whole config directory being copied, since **the key is in it** — nor anything else running as your user |

So **keep the config directory out of version control, syncing folders and backups.** To guard against the whole directory being copied, put the key elsewhere with `SSHU__KEY_FILE` — and back that key up yourself, because losing it loses every stored password.

A password is never displayed, and it reaches `ssh` through `SSH_ASKPASS`, never through an environment variable or the command line. If you would rather sshu held no password at all, use `auth: privatekey` (stores only a path) or `auth: sshconfig` (stores nothing).

### Host keys

- **ssh tab** — runs the real `ssh`, so host keys are handled by OpenSSH with your `~/.ssh/config` and `known_hosts`.
- **file transfer tab** — refuses an unknown host and a changed key rather than asking. Accept a new host by connecting to it once from the ssh tab, or with `A` under manage → KnownHosts. (`auth: sshconfig` hosts go through the real `ssh` here too, and get OpenSSH's own prompt.)

## Known limitations

- The file transfer tab cannot confirm an unknown host key interactively for password, privatekey or credential hosts — accept it through the ssh tab first.
- Passphrase-protected private keys do not work on the file transfer tab for `privatekey` hosts; use an `auth: sshconfig` host, which goes through `ssh` and the agent.
- No content search on remote files.
- No mouse support, no automatic reload when `hosts.yaml` changes on disk, no saved sessions, no keychain storage for passwords.

## Links

- [CHANGELOG.md](CHANGELOG.md) — what changed in each release
- [`docs/dev-remarks.md`](docs/dev-remarks.md) — the developer's notes: how it works, why, the design docs, building and testing

## terminu family

sshu follows the [terminu design principle](https://github.com/vulcanshen/terminu/tree/v0.1.21/principle): the same keys and the same menus as the rest of the family — [kbu](https://github.com/vulcanshen/kbu) (Kubernetes), [filu](https://github.com/vulcanshen/filu) (files), [webu](https://github.com/vulcanshen/webu) (the web) and [locku](https://github.com/vulcanshen/locku) (screen lock).

## License

[GPL-3.0](LICENSE)
