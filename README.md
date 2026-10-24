# sshu

<p align="center"><img src="docs/icon.svg" width="128" alt="sshu icon" /></p>

[![GitHub Release](https://img.shields.io/github/v/release/vulcanshen/sshu)](https://github.com/vulcanshen/sshu/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/vulcanshen/sshu)](https://go.dev/)
[![License](https://img.shields.io/badge/license-GPL--3.0-blue)](LICENSE)
[![Charm in the Wild](https://img.shields.io/static/v1?label=Listed%20in&message=Charm%20in%20the%20Wild&color=6B5CE7)](https://github.com/charm-and-friends/charm-in-the-wild#networking-and-file-transfer)

**Language**: English · [繁體中文](README-zh_TW.md)

**An ssh and sftp front end for the terminal.** Keep your hosts in one file, open as many live shells as you like side by side, and move files between any two machines. `Tab`, `Enter`, `Esc`, `Space` and `?` drive everything: no hotkeys to memorize, no setup.

> _When in doubt, press_ **`Space`**.

Inspired by [Termius](https://termius.com/) — hosts, sessions and transfers under one roof — but in your terminal.

## Demo

![ssh grid](docs/demo-grid.gif)

## Features

- **Nothing to memorize** — `Space` on any panel lists everything you can do there; every letter hotkey is a row of that menu, and `?` lists the keys.
- **A grid of live ssh sessions** — each cell is a real `ssh` in its own terminal, as many as you like, arranged across, down or in columns. `PgUp` goes back through a session's history, `Alt-v` selects and copies out of it, `Alt-z` gives one cell the whole grid, and `Alt-Enter` hands a sshu nested in a cell every key.
- **Transfers between any two machines** — local ↔ remote ↔ remote on one screen. Search a whole subtree, read a file without fetching it, or open it in your own `$EDITOR` and have it written back.
- **Your `~/.ssh` files, in place** — browse and edit `~/.ssh/config` and `~/.ssh/known_hosts` without losing a comment. An `auth: sshconfig` host stores nothing and lets `~/.ssh/config` decide everything — ProxyJump, the agent, keys.
- **Reusable credentials** — define a user and how it authenticates once, use it from any number of hosts.
- **Tags** — your own words on a host, found by the filter: type `prod` and the whole group is there.
- **Failures that explain themselves** — a session that ends badly says what ssh said, and `!` opens the App Log: what sshu did and what went wrong since it started, with everything the far end said. Manage keeps a record of every connection and every change you made.
- **Passwords are encrypted on disk** — and never shown, and never passed through the environment.

## Install

> sshu runs on **macOS and Linux** (it uses a Unix PTY); there is no Windows build — use WSL.

**Homebrew** (macOS / Linux):

```bash
brew install vulcanshen/tap/sshu
```

**Install script** (puts the latest release binary in `~/.local/bin`, or `/usr/local/bin` as root):

```bash
curl -fsSL https://raw.githubusercontent.com/vulcanshen/sshu/main/install.sh | sh
```

Building from source is in [`docs/dev-remarks.md`](docs/dev-remarks.md).

### Requirements

- **A Nerd Font** — required, not optional: auth methods, file types and marks are Nerd Font glyphs. sshu asks the terminal at start how many cells an icon and a powerline symbol take; when it guesses wrong, set it yourself (`1` or `2`):

  | Variable | Read by |
  |---|---|
  | `SSHU__ICON_WIDTH`, `SSHU__POWERLINE_WIDTH` | sshu only |
  | `TERMINU__ICON_WIDTH`, `TERMINU__POWERLINE_WIDTH` | every app of the [terminu family](#terminu-family) — set once, every app reads it; inside a family app's terminal, the outer app sets them for you |

  Over ssh, sshu hands its own two numbers on as `LC_TERMINU_ICON_WIDTH` and `LC_TERMINU_POWERLINE_WIDTH` (sshd accepts `LC_*` by default), so a family app on the far side draws right too.

- **At least 60×20** — on a smaller terminal sshu shows only its name and how big it needs to be, and comes back as it was when the window grows.
- **A truecolor terminal** (24-bit colour) — the soft colours, the layers of popups and the dimmed screen under them cannot be told apart in 256 colours.
- **A terminal that sends Alt** — sshu's `Alt-…` keys (`Alt-Esc` to leave a session, `Alt-z`, the `Alt`-arrows) need Option to act as Meta: in macOS Terminal turn on "Use Option as Meta key", in iTerm2 set Option to *Esc+*; kitty, Alacritty and WezTerm send it already.
- **OpenSSH's `ssh`** for the SSH screen (every macOS and Linux has it). File transfer speaks SFTP itself.

### First start

sshu opens on Manage → Hosts with an empty list, and makes `~/.sshu` and the key that will encrypt your passwords (see [Where your data lives](#where-your-data-lives)).

Coming from an older sshu, what it kept elsewhere moves on the first start: on macOS the settings directory from `~/Library/Application Support/sshu` to `~/.config/sshu`, and the hosts, the credentials, the key that encrypts them and the journals from the settings directory to `~/.sshu`; the key goes with the files it encrypts, or all of them stay. It moves item by item, and a message says from where to where. What the new place already has stays in the old one, and what cannot be moved stays in use where it is: sshu shows both paths, for you to merge or move it yourself.

### Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/vulcanshen/sshu/main/uninstall.sh | sh
```

It removes the binary, then asks before touching the settings directory, and the data directory — your hosts and credentials are there.

## Quick start

```bash
sshu
```

Press `A` to add a first host, then `Enter` on it to connect. `F` goes to file transfer, `S` to the grid of sessions. When you don't know what to do next, press `Space`.

## Usage

Three screens, switched with `M`, `F` and `S` (or from `Space` → Global operation):

```
 [M]anage  [F]ile transfer  [S]SH
```

The keys that work everywhere:

| Key | What it does |
|---|---|
| `Space` | What can I do here: the focused panel's menu; its last row opens the global operations (switch screens, quit) |
| `?` | The keys of whatever is in front |
| `!` | The App Log: what sshu did and what went wrong since it started, newest last; the footer says when there are errors you have not seen |
| `Tab`, `1`–`9` | The next panel, or panel `[N]` of this screen |
| `Enter` | The obvious thing: connect, open, go in |
| `Esc` | Back one step — close the top popup, clear a filter, up a directory |
| `j\|k`, `u\|d`, `gg\|G` | Down and up, half a page, to the top and the bottom (arrows too) |
| `q\|Ctrl-C` | Quit; with sessions or transfers running it asks first, and a second `Ctrl-C` leaves at once |

Every letter hotkey below is also a row of its panel's `Space` menu. **The case printed in brackets is the key**: `[A]dd` is `Shift-A`, `[t]ransfer` is a plain `t`.

### Manage

`[1]` lists what sshu keeps — **SSHU**: Hosts, Credentials; **SSH**: Config (`~/.ssh/config`), KnownHosts (`~/.ssh/known_hosts`); **Logs**: Connections, Changes. `[2]` shows the one under the cursor; `Enter` or `2` moves there.

| Key | What it does |
|---|---|
| `Enter` | On a host: what it connects to and how, then `Enter` again connects. Elsewhere: read it whole (passwords are always masked) |
| `A` | Add a host, a credential, a `Host` block, or a known host key (sshu asks the host for its key and shows the fingerprint before anything is trusted) |
| `E` | Edit the row under the cursor |
| `D` | Duplicate: a new one filled in from this row |
| `X` | Delete (asks first) |
| `/` | Filter the hosts: name, user, host, port and tags at once |
| `C` | Connections, Changes: clear that journal (asks first) |

In a form, `j|k` move between fields, `Enter` changes the one under the cursor, and `Tab` goes to the buttons.

A `Host` block's form has fields for `HostName`, `User`, `Port` and `IdentityFile`; the block's other keywords are listed under *Others*. The *Edit file* button opens the file in your `$EDITOR` over the form, on that block's line; with changes in the form it asks to save them first, since the editor opens the file as it is on disk. When the editor closes, the form shows what the file now says.

### File transfer

Two sides, each this machine or a saved host: files on top, what you marked below — `[1]` `[2]` on the left, `[3]` `[4]` on the right. `h|l` cross to the other side. `local` opens in the directory you started sshu in, so `cd ~/release && sshu` starts right there.

| Key | What it does |
|---|---|
| `Enter` | Into a directory; on a file, read it |
| `Esc` | Up one directory |
| `a` | Mark / unmark |
| `t` | Send it to the other side's directory |
| `r` | Rename |
| `v` | Read it — text coloured, binary as hex, a directory as its listing |
| `e` | Edit in `$EDITOR`; a remote file is fetched and written back. `Alt-Esc` in the editor abandons the edit (asks first) |
| `x` | Delete it (asks first) |
| `/` | Search the whole tree under this directory, the one under the cursor previewed beside it; `Enter` on a result goes there |
| `A` | Add: `name` makes a file, `name/` a directory |
| `R` | Read this directory again |
| `H` | Pick this side's host (`local` first) |
| `D` | Disconnect this side, or call off a connection on its way |

On a marks panel, `Enter` opens the mark (a directory in the files panel, a file to read), `r`, `v`, `e`, `t` and `x` work on the mark under the cursor, `c` forgets it (nothing on disk changes), and three keys work on every mark of that side: `T` sends them all, `X` deletes them all (asks first), `C` forgets them all. `J`, anywhere, lists the transfers of this run: `Enter` cancels a running one or shows the whole of one that ended.

What cannot run right now is dimmed in the menu and its key does nothing: `H` and `D` while a transfer runs, `a` on a file still arriving, `e` on a directory, `t` and `T` while the other side has no host.

### SSH

`[1]` the live sessions, `[2]` how the grid is laid out, then one panel per session from `[3]` on. `Enter` on a session (or on its cell) puts the keyboard in it; from then on every key belongs to the remote, but these:

| Key | What it does |
|---|---|
| `Alt-Esc` | Take the keyboard back out (asks first: two quick `Esc` in vim can read as `Alt-Esc`; `Esc` goes back in) |
| `Alt-←\|→\|↑\|↓` | The cell that way |
| `Alt-z` | This cell over the whole grid, and back |
| `Alt-v` | Select in the cell, `v` to start, `y` to copy, `Esc` to drop the selection; `Space` lists the keys |
| `Alt-Enter` | Lock: every key goes to the cell, `Alt-Esc` too, for a sshu nested in it; `Alt-Enter` is then the way out, and leaving unlocks it |
| `PgUp` | The session's history: `j\|k`, `u\|d`, `gg\|G`, `PgUp\|PgDn` scroll, `Esc` back to live, `Space` lists the keys (any other key goes to the session) |

A cell still connecting says so, and what you type waits until ssh is through.

On `[1]`: `H` hides a session's cell from the grid or shows it again, `C` closes it (asks first), `D` opens another session to the same host; *Close all sessions* is in the `Space` menu. On `[2]`, choose horizontal, vertical or custom, and the number of columns for custom.

## Settings

### Where your data lives

Two directories: sshu's own settings, and your data — the hosts and credentials you wrote, and the journals sshu keeps.

| The settings directory | |
|---|---|
| `$SSHU__CONFIG` | when set |
| `$XDG_CONFIG_HOME/sshu` | when that is set |
| `~/.config/sshu` | otherwise, on every platform |

The data goes in `~/.sshu` (`$SSHU__DATA` when set).

| File | Where | What is in it |
|---|---|---|
| `hosts.yaml` | `~/.sshu` | your hosts |
| `credentials.yaml` | `~/.sshu` | reusable identities — a name, a user and how it authenticates; a host uses one with `auth: credential` and `credential: <name>` |
| `config.yaml` | settings | settings (optional, below) |
| `state.yaml` | settings (`$SSHU__STATE` moves it) | how the SSH grid is arranged; sshu writes it, and deleting it only brings back the default |
| `.sshukey` | `~/.sshu` | the key that encrypts stored passwords |
| `connections.yaml` · `changes.yaml` | `~/.sshu` | the two journals |

All plain YAML you can edit by hand, kept at `0600`.

A file of these sshu cannot read — a typo in `hosts.yaml`, a file it may not open — is asked about when sshu starts: `Enter` renames it to `hosts.yaml.bak` (`.bak.1` and on when that is taken, never over one) and sshu starts as it did the first time, `Esc` leaves sshu and prints which file and why. A key that will not read is asked about on its own: the passwords it encrypted stay as they are, for the old key to open again.

### Settings — `config.yaml`

Optional; sshu reads it and never writes it, so your comments and layout stay. Leave out what you don't want changed.

```yaml
# How long one connection attempt may take, in seconds (1–600). Default 15.
connect_timeout: 15
# Shared by the terminu family: in the identity file picker's preview, show
# only the first line of a private key and nothing of a file only you may read.
# Default false.
hide_secrets_in_preview: false
# Shared by the terminu family: how many cells a Tab takes in a file you read
# or a preview. Default 4.
input_tab_width: 4
```

### Passwords

Stored passwords are encrypted with AES-256-GCM, so the `password:` field reads `ENC:…`. The key is `.sshukey` in the same directory, made on first start; set `SSHU__KEY_FILE` to keep it elsewhere.

|  |  |
|---|---|
| **Protects against** | `hosts.yaml` or `credentials.yaml` leaking on its own — pasted into a chat, committed by mistake, swept up by a backup |
| **Does not protect against** | the whole data directory being copied, because **the key is in it**; or any other program running as you |

So keep `~/.sshu` **out of version control, out of synced folders and out of backups.** To survive the whole directory being copied, move the key with `SSHU__KEY_FILE` — and back that key up yourself: lose it and the stored passwords go with it.

Passwords are never drawn on screen, and reach `ssh` through `SSH_ASKPASS`, never the environment or the command line. If you'd rather sshu stored none, use `auth: privatekey` (a path only) or `auth: sshconfig` (nothing at all).

### Host keys

- **SSH screen** — runs the real `ssh`, so host keys are OpenSSH's business, with your `~/.ssh/config` and `known_hosts`.
- **File transfer** — an unknown host or a changed key is refused, not asked about. Connect once from the SSH screen, or press `A` in Manage → KnownHosts. (An `auth: sshconfig` host goes through the real `ssh` here too, with OpenSSH's own questions.)

## Limitations

- File transfer cannot confirm an unknown host key for password, privatekey or credential hosts — accept it from the SSH screen first.
- File transfer cannot use a private key with a passphrase on a `privatekey` host; use an `auth: sshconfig` host, which goes through `ssh` and the agent.
- No search inside remote files' contents.
- No mouse; `hosts.yaml` changed on disk is not reread while sshu runs; sessions are not saved; passwords are not kept in the keychain.

## Links

- [CHANGELOG.md](CHANGELOG.md) — what changed in each release
- [`docs/dev-remarks.md`](docs/dev-remarks.md) — developer notes: how it works, why, building and testing

## terminu family

sshu follows the [terminu design principle](https://github.com/vulcanshen/terminu/tree/v0.9.14/principle) and is built on [terminu-go](https://github.com/vulcanshen/terminu-go): the same keys and the same menus as the rest of the family — [kbu](https://github.com/vulcanshen/kbu) (Kubernetes), [filu](https://github.com/vulcanshen/filu) (files), [webu](https://github.com/vulcanshen/webu) (the web), [locku](https://github.com/vulcanshen/locku) (a screen lock), [kamonu](https://github.com/vulcanshen/kamonu) (the family's marks) and [ccdrawu](https://github.com/vulcanshen/ccdrawu) (a sidebar for Claude Code).

## License

[GPL-3.0](LICENSE)
