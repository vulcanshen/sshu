# Changelog

## [1.5.0] — 2026-09-09

The two files in `~/.ssh` that were already steering sshu are panels now, and
sshu running inside sshu — which is what happens the moment you install it on
the server too — stopped being a thing you fight.

Changes since 1.4.2.

**Breaking: `Alt+Enter` no longer zooms — zoom is `Alt+Z`.** The layer key
needed the most reliable chord on the keyboard, because once a cell is locked
`Alt+Enter` is the only key that can unlock it: zoom failing costs a nicety,
the layer key failing traps you. `Alt+Z` is what tmux calls zoom. If a window
manager on your machine already claims it (AeroSpace does), unbind it there.

The demo GIFs are from 1.4.2 and do not show any of this yet.

### Added

- **`~/.ssh/config` is now a panel: manage → SSH → Config.** It is the one file
  sshu manages that is not sshu's, and it earned a panel because it was already
  running half the app invisibly — tab `[3]` launches the real `ssh`, which
  reads this file, so `HostName`, `ProxyJump` and everything else in it already
  decide what a session does. (Tab `[2]`/`[4]` speak SFTP themselves and read
  none of it, which is a real asymmetry the panel makes visible.)

  One row per `Host` block, `Enter` for the whole of it, `[E]dit` /
  `[D]uplicate` / `[X]` / `[A]dd`.

  ```
   Host          HostName                User      File
   office        10.20.0.1               staff     ~/.ssh/config.d/work
   *                                                 ~/.ssh/config
   gw            198.51.100.10           vulcan    ~/.ssh/config
   prod          prod.example.internal   deploy    ~/.ssh/config
  ```

  The `File` column appears only when `Include` actually brought in a second
  file, and it carries the WHOLE path, cut from the front — two included files
  both called `hosts` are exactly what it exists to tell apart.

  **Editing a block changes only that block's lines.** Comments, `Match`
  blocks, the global options above the first `Host`, and every keyword sshu has
  never heard of come back byte-for-byte — including the indentation and the
  `Key=Value` separator you used. A change to one value is a one-line diff.

  The form carries whatever the block actually holds: five permanent rows
  (`Host`, `HostName`, `User`, `Port`, `IdentityFile`), then one row per
  keyword already in the block, then `+ add option`. That is what makes a
  `Host *` block editable — the shape every macOS config has, whose contents
  are entirely outside those five.

  ```
  ╭─  Edit Host block ─────────────────╮
  │  Host              *                │
  │  HostName                           │
  │  User                               │
  │  Port                               │
  │  IdentityFile                       │
  │  ─ other options ─────────────────  │
  │  AddKeysToAgent    yes              │
  │  UseKeychain       yes              │
  │  + add option                       │
  ╰─ Tab next  Enter save  Esc cancel ──╯
  ```

  Details that follow from the file being somebody else's:

  - **A repeated `Host` pattern is kept.** ssh takes the first value it finds
    for each keyword, so two `Host prod` blocks both apply — the opposite of
    `hosts.yaml`, where a repeat is unreachable and gets dropped on load.
  - **Clearing a field removes its line.** There is no keyword-without-argument
    in this file, so that is the only gesture that could mean "take this out".
  - **`Include` is followed.** Its blocks are listed and editable like any
    other; they just live in another file, and every question about them says
    which — the `File` column when the tree spans more than one, the whole
    path in the detail float, and the file's own name in the delete
    confirmation. A save writes only the files that changed. `[A]` always
    writes to `~/.ssh/config` itself, because an include tree has no obvious
    home for something new. An Include that resolves to nothing is still
    disclosed, by name.
  - **A save is refused if another editor got there first**, and the refusal
    puts their file on screen — sshu rewrites the whole file, and vim, VS Code
    and a dotfiles pull all write here too.
  - **The file keeps the mode it had.** ssh only objects when others can *write*
    it; a config deliberately left at 0644 is not sshu's to narrow. A file sshu
    creates is 0600.

- **`~/.ssh/known_hosts` is a panel too: manage → SSH → KnownHosts.** This is
  the file with teeth — sshu already verifies against it and **refuses a
  connection outright** when a known host's key has changed, deliberately
  without offering a yes/no. Until now the only way out of that refusal was
  `ssh-keygen -R` in another terminal.

  ```
   Host                  Type        Fingerprint
   [198.51.100.10]:2222  ed25519     xK9vQ2mF7pLc…
   gw.example.com        rsa         a4Tz1nHb0sWd…
   @revoked old.corp     ed25519     Rr8kM3vY6qEu…
   (hashed)              ecdsa-256   Lp2wN9xJ4tZa…
  ```

  Same line-preserving discipline as Config, and it matters more: a real
  known_hosts is mostly annotation. One entry is one line; a rename rewrites
  that line's first field and nothing else.

  - **`[A]` fetches the key instead of asking you to type it.** It opens the
    connection far enough to see the host key, **stops before authenticating**,
    and shows the SHA256 fingerprint. Nothing is written until you accept — the
    same shape as ssh's own first-connect question, and for the same reason.
  - **`[E]` changes which names a key is trusted for.** The key itself is never
    editable: retyping 68 characters of base64 is not an edit, it is a new key,
    and that is what `[A]` is for.
  - **`[X]` is the one you came for.** It names the fingerprint and says the
    next connection there will ask again.
  - **There is no `[D]uplicate`**, and that is deliberate: it would mean
    trusting one key under a second name, which the comma-separated name field
    already does in one line.
  - **Deleting an entry leaves the comment above it** — the opposite of the
    Config panel. A comment over a `Host` block titles that block; a comment in
    known_hosts sits over a *run* of entries.
  - **Hashed names say so.** `|1|…` is an HMAC and the hostname cannot be read
    back from the file; the row shows `(hashed)` and the status line counts
    them, so those cells do not read as a rendering fault.
  - **`@revoked` rides in front of the name, in the warning colour.** A row that
    reads as trusted when it is the opposite is the one mistake this panel must
    not make.
  - **A save is refused if ssh got there first** — which is routine here, since
    ssh appends on every first connect — and the refusal puts the file as it now
    stands on screen.

- **A host's detail now says what `~/.ssh/config` will actually do to it.**
  sshu passes `hosts.yaml`'s fields to ssh as command-line flags, and those
  outrank the file — so `Port` is sshu's while `HostName` is the file's, and a
  host whose `host` field is a config alias lands somewhere neither of them
  said on its own.

  ```
  │ ~/.ssh/config · Host *                            │
  │  AddKeysToAgent         yes                       │
  │  StrictHostKeyChecking  ask                       │
  │                                                   │
  │ ~/.ssh/config · Host gw                           │
  │  HostName               198.51.100.10             │
  │  Port                   2222 — sshu sends -p 22   │
  │                                                   │
  │ ~/.ssh/config · not shown                         │
  │  Include                1 file, not read          │
  ```

  It is the UNION of every matching block with the first value winning,
  because that is what ssh does — a host normally matches several. One
  section per contributing block, in file order, which is precedence order.
  A value sshu overrides is marked only when the two actually differ, a value
  a later block would have shadowed is not listed at all, the whole thing is
  absent when nothing matches, and it discloses the `Include` and `Match`
  sources it could not read.

- **Deleting a `Host` block warns which sshu hosts it breaks** — "1 sshu host
  resolves through it and will not connect" when the block supplies their
  `HostName`, or the milder "2 sshu hosts inherit options from it" otherwise.

- **A sshu reached over ssh no longer loses its colours.** ssh forwards TERM
  and nothing else, so the far side decided it had 256 colours and quantised
  every colour in its UI — which is why a nested sshu's popups looked subtly
  wrong next to the outer one's. The depth now travels with the connection.

- **And the outermost sshu can now act on any layer it can see.** Pick an
  inner layer in the lock menu and it locks or releases, however deep it is:
  the command carries a hop count, every layer subtracts one and passes it
  on, and the layer that receives zero performs it. Nothing opens anywhere
  but the outermost menu and nobody counts. No acknowledgement is asked for
  — the target's new state comes back on its next frame, so the row changes
  once the change is real rather than when the key was pressed.

- **A nested sshu now tells the one outside it what is going on.** The lock
  menu lists the whole chain — every layer and whether it is passing keys
  through — so the depths are on screen instead of in your head. It rides in
  the output stream the way a program has always set a terminal title:
  invisible to anything that does not recognise it, verified silent against
  tmux and against sshu's own emulator. Nobody has to know how deep they
  are — each sshu reports itself and passes on what it heard — so there is
  no environment variable to forward and nothing to configure on the far
  side. A sshu too old to speak simply says nothing, and the layer above it
  behaves exactly as it does today.

- **Nested sshu works now: Alt+Enter locks a cell into a transparent pipe.**
  Run sshu on the server too — the reason to nest is that installing sshu is
  one command, while ProxyJump is the config file you were avoiding — and the
  outer sshu used to eat the whole Alt namespace, leaving the inner one
  unusable. Alt+Enter opens a two-row menu, `[L]ock / [R]elease`; locked,
  every key passes through, so the inner sshu's Alt+Esc, Alt+arrows and
  selection mode all work. The chord also forwards itself, so in a chain it
  opens the menu at every layer, each naming its own state — deal with them
  outside-in, no counting. A locked cell wears a lock in its title and its
  footer offers only the release, because every other entry would be a lie.
  Verified end-to-end against a real sshd: the chords arrive byte-for-byte.

### Changed

- **Zoom moved from `Alt+Enter` to `Alt+Z`** (tmux's own zoom key). The layer
  chord had to be the most bulletproof one on the keyboard — ESC+CR passes
  every terminal and every ssh hop verbatim — because once a cell is locked,
  Alt+Enter is the only key that can unlock it: zoom failing costs a nicety,
  the layer key failing traps you.

- The `[M]anage` nav's second category is **Others** rather than Events. It was
  a label for the one row under it rather than for a category, and it stopped
  being true the moment anything that is not an event needed somewhere to live.

## [1.4.2] — 2026-09-08

Enter on a row now shows you the row and offers the thing Enter was for, in one
float instead of two — and a name that appears twice in a hand-edited file stops
quietly breaking the list it is in.

Changes since 1.4.1. All three demos are re-recorded on this build.

### Changed

- **`[V]iew` is gone; Enter does both jobs.** On a host, Enter opened a
  confirmation that said the address and the auth method — every word of which
  the `[V]iew` float already said, and said better. They were two read-only
  floats over the same row, and you had to close one to open the other.

  Enter now opens the detail itself, with the question at the foot of it:

  ```
  ╭─ 󰛐 staging-api ─────────────────────╮
  │ Connection                          │
  │  Name           staging-api         │
  │  Host           staging.example.com │
  │  Port           22                  │
  │                                     │
  │ Auth                                │
  │  Type           credential          │
  │  Credential     shared-deploy       │
  │  User           deploy              │
  │  Identity file  ~/.ssh/id_ed25519   │
  │                                     │
  │ Connect to "staging-api"?           │
  │                                     │
  ╰─ Enter connect  Esc close ──────────╯
  ```

  Only wanted to look? Esc. Credentials get the same shape with their own verb —
  `Edit "<name>"?` — and `[E]dit` still goes straight to the form, so looking at
  a credential no longer means opening a form you can type into.

  A host whose credential is missing is shown but not offered: the red row names
  the credential and says it cannot connect, and there is no connect line to
  press. That replaces a toast which appeared over a float that never opened.

- **`V` is the logo again, everywhere.** It used to stand aside wherever a panel
  had a real `[V]iew`, which meant the letter meant one thing on two panels of
  the manage tab, the logo everywhere else, and flipped between them on the same
  panel depending on whether the table was empty. Nothing claims it now.

### Fixed

- **A name repeated in a hand-edited `hosts.yaml` or `credentials.yaml` no
  longer breaks the list.** Only hand-editing could produce one — the form and
  the save path both refuse duplicates — and what came back was a list that
  behaved as if it had none: every lookup resolved to the first entry, so the
  second was on screen but unreachable; deleting it removed both rows at once;
  and until the file was fixed by hand, nothing could be saved at all.

  Both files now drop the repeats as they load, keeping the first of each name —
  the one every lookup already resolved to. The file itself is left alone until
  you save something, and the app log says what was dropped, by name:

  ```
  hosts.yaml: ignored 2 duplicate entries ("prod", "prod"); the first of each name is the one in use
  ```

## [1.4.1] — 2026-09-07

An sshu opened inside an sshu starts at once instead of five seconds late, and
the hosts table stops making its one genuinely fixed column look like the one
that moves.

Changes since 1.4.0:

### Fixed

- **A pty cell answers the questions it is asked.** Anything started in a cell
  sat for five seconds before drawing its first frame — an sshu inside an sshu
  most visibly, but every Bubble Tea program and anything else that asks the
  terminal a question paid the same.

  A terminal is not only a screen being written to; it is also asked things —
  where the cursor is, what the background colour is — and the asker writes its
  query and then **blocks on the answer**. The emulator behind a cell knew both
  questions and produced correct answers, but its replies were being written to
  a discard: nothing ever reached the child, which waited out its own five
  second timeout and carried on with a guess. For a Bubble Tea program that
  happens in package init, before `main` runs, so the delay landed before the
  program could put anything on screen at all.

  The emulator's replies now go back up the pty to the child, which is what a
  terminal does. Measured on `sshu version` driven through a cell: 5.04s before,
  0.06s after.

### Changed

- **Port is left-aligned, and Auth has room for a credential's name.** Port was
  the only right-aligned column in the hosts table, so a short port sat behind a
  gap that grew and shrank with its length — which made the one column whose
  width is genuinely fixed look like the one changing size. It is left now, like
  every other column. Right-aligning numbers is the habit from columns you add
  up, and nobody adds up ports.

  Auth was sized by the longest method name, which is the wrong measure: a host
  using a credential shows **which** credential, and that is a name the user
  chose — the only cell in the table whose length nobody can bound. It gets four
  more columns. The credentials list keeps the old width, because its own Auth
  cell only ever holds a method name; the two columns were never the same
  column, they just happened to be the same size.

  Name is now the widest column rather than the second widest, which is what the
  table was always meant to be — it is what a host is picked by. The cost is
  disclosed: Auth needs four more columns to survive, so it is dropped below 55
  columns of terminal rather than 51.

  Fixing the widths surfaced a rounding bug older than the change: after the
  minimums bite, the shares could total more than the row had, and the leftover
  cell pushed the panel's right border out of line. It never fired at the
  previous weights.

## [1.4.0] — 2026-09-04

A pty cell you can copy out of without a mouse, a form whose Enter means one
thing, and a `clear` that stops looking like it broke the scrollback.

Changes since 1.3.0:

### Added

- **Selection mode on a pty cell (`Alt+v`)** — freeze the cell, move a cursor
  over the frozen text, and copy what you sweep to the system clipboard.

  The terminal's own selection is the obvious way to get output out of a panel,
  and a grid is exactly what breaks it: a drag runs along a physical screen
  line, so it collects the border and the neighbouring cell's output on the way
  past. Turning the mouse on to take the drag over would have cost native
  selection in the **whole** app — the lists and popups included, where it still
  works fine — so this is a keyboard mode instead, and it costs nothing outside
  itself.

  The keys are vim's: `hjkl` moves and scrolls the page at the edges, `u`/`d`
  go half a screen, `v` and `V` select by cell or by line, `y` copies and
  leaves. With nothing selected `y` takes the cursor's line, which puts "copy
  what just printed" on three keys. `Esc` drops the selection first and the
  mode second; `Alt+v` again, or `Alt+Esc`, leaves outright.

  The cell's frame turns yellow while the mode is up, and the footer swaps to
  the mode's own keys — every key under your fingers means something different
  in there. The text goes to the **system** clipboard via `pbcopy`, `wl-copy`,
  `xclip` or `xsel`, so it pastes into a local editor with no help from the
  terminal, and the result is always reported: how many lines went, or what to
  install if none of the tools were there.

### Changed

- **A form's `Enter` asks one question: is this finished?** It used to mean
  three different things depending on which row the cursor happened to be on —
  browse, choose, or submit — and the only way to tell them apart before
  pressing was to remember where you were.

  Finished, `Enter` saves. Not finished, it is "next" and loops, so holding
  `Enter` walks the form and then submits it. What counts as finished follows
  the Auth toggle, because it is exactly the rows Auth has left lit: `password`
  wants a Password, `privatekey` wants an IdentityFile, `credential` wants a
  Credential and stops wanting a User. The popup's hint says which of the two
  `Enter` is right now — `next` or `save` — and flips the moment the last field
  is filled.

  The empty IdentityFile and Credential rows keep their chooser: "next" there
  would step over the only row that cannot be filled any other way.

  One consequence worth knowing: a `privatekey` host with **no** key file used
  to be savable, and meant "let ssh's own config and agent decide". The form
  will not create one any more. Hand-written `hosts.yaml` is unaffected — the
  store's validation did not change.

- **`[D]uplicate` on hosts and credentials, and `D` stops meaning two things.**
  Delete moves to `X`. That is not a scramble for a free letter: `D` already
  meant Duplicate in the sessions list and `x`/`X` already meant delete in the
  file transfer tab, so manage's `D` was the one that agreed with neither. Now
  `D` is Duplicate everywhere and `X` is delete everywhere. The wrong-key
  accident also runs the safe way round — a hand that expected Delete gets a
  form it can Escape out of, rather than the reverse.

  Duplicate opens an **Add form already holding the whole row**, cursor on Name.
  Nothing is written and no `-copy` suffix is invented. Every field has a value,
  so `Enter` means save; the name it arrives holding belongs to the row it was
  copied from, so that save is refused on the Name field — which is where the
  cursor already is. Forcing the rename is the uniqueness check doing its
  ordinary job, not a special case written for duplication.

- **A refused form is said once, in the form.** The credential form used to
  raise a toast on top of its own error row; the host form never did. The row
  already names the field, marks it, and puts the cursor on it, and a second
  copy of that sentence floating over the popup that is displaying it is not
  emphasis. Both forms refuse the same way now.

- **Credentials: `[E]dit` has a letter.** It was reachable only by `Enter`,
  which prints no bracket, so the only way to learn it was to press `Enter` and
  see what happened. `E` opens it now, and `Enter` still does too — on a
  credential, "go in" and "edit" are the same door.

### Fixed

- **Output arriving in the same read as a `clear` is no longer dropped from the
  scrollback.** A remote erasing its history (`\x1b[3J`) discarded the whole
  chunk it arrived in — and `clear && ls` is one chunk, so the listing was never
  filed. The panel then correctly reported having no history to page through,
  which from the outside looks exactly like `clear` breaking `PgUp`. An erase
  now clears what came before it and keeps what came after.

## [1.3.0] — 2026-09-03

A read-only `[V]iew` for the rows that hold secrets, a sessions list that says
what you *called* a machine as well as what it is, and two things that were
asking questions they did not intend to answer.

Changes since 1.2.0:

### Added

- **`[V]iew` on hosts and credentials** — a read-only popup for what a row
  actually holds. The table cannot answer it: columns are shed as the terminal
  narrows, the auth column is a glyph rather than a word, and the password can
  never be on a table at all. Opening the edit form to look was the alternative,
  and a form is a thing you can accidentally change.

  A host is shown as its connection and its auth, in two sections. A stored
  password is reported as a **fixed-width mask** — not one bullet per character
  the way the form draws it, because in the form the length is your own and here
  it is somebody else's. A privatekey host shows the path, since the path is all
  it has. A credential host is resolved on the spot: the name it points at, then
  the user and secret that name supplies — and when the credential is gone, the
  row says so and says what it costs ("missing, cannot connect"). The credential
  popup is the auth half alone, which is the whole of what a credential is.

  `V` was the u-family easter egg. The egg is now the lowest claim on the key in
  the app: a panel with a real `[V]iew` takes it, and the logo gets what is left
  — including on an empty table, where there is nothing to view.

### Changed

- **A session on the list is two lines, and says its name** — `<name>` above,
  `<user>@<host>:<port>` below. One line could hold only one of the two, and the
  one it held was the address, so a list of sessions never showed the names the
  hosts table is entirely made of. Neither line wraps: the name is cut, the
  address shortens on each side of a kept `@`, and the port never gives. The
  address starts at the border rather than lining up under the name — the
  alignment reads better and costs the address two columns on every row, which
  the panel then has to be two columns wider to give back. The left column went
  30 → 26 with the columns that freed.

- **`Tab` on the sessions list is also `[H]ide`** — the row used to be called
  "Display", a noun among verbs, named for the direction it almost never runs
  in: a session's cell goes onto the grid the moment it connects. The label says
  `Hide` and the hint says `hide/show`, because a label is what you scan and a
  hint is what you read when you are not sure. `Tab` still does it — it is this
  tab's own key, disclosed where panel keys are disclosed.

- **The custom grid asks for one number, the column count** — it used to ask for
  rows × columns, and the rows half was very nearly a lie: the grid returned
  `max(rows, ceil(n/cols))`, so asking for 2×3 and opening ten sessions gave a
  2×5 grid. The stated row count only ever mattered when there were too *few*
  cells to fill it, where all it bought was reserved empty space. `"2x3"` is now
  refused rather than mined for a digit: that is an answer to the old question,
  and quietly taking the 2 would apply a shape nobody chose.

### Removed

- **The `#N` on two sessions to one host.** It keyed on the hosts.yaml *name*
  while every row and cell title drew the *address*, so it tagged the wrong
  pairs in both directions: two entries pointing at one box drew identical rows
  and got no tag at all, while one entry edited between two connects got #1/#2
  on two different machines. Two sessions to one host are two entries, in the
  order they were opened.

## [1.2.0] — 2026-09-03

The tabs come off their Alt chords: one shifted letter each, **`M` / `F` / `S`**.
Three keys move out of the way to make room, and one of them was overdue for a
rename anyway. Inside a pty the terminal now keeps a history you can page
through, and two keys that were never sshu's to take have been handed back to
the remote — starting with `Ctrl+C`, which used to quit the whole app from
inside a session.

Changes since 1.1.0:

### Added

- **Terminal history in the ssh grid — `PgUp` / `PgDown`** — the embedded
  emulator is a fixed grid and clears the rows that leave the top, so anything
  that scrolled past was not merely unreachable, it was gone. Every chunk read
  from the PTY is now split into lines and filed as it goes in, colours and all,
  up to 10 000 lines per session. A cell showing history says so in its title
  (`󰋚` and how far back), because a cell showing the past and a cell whose
  remote has gone quiet are otherwise the same still picture. Typing snaps back
  to live.

  The keys are **borrowed, not taken**: a full-screen program pages with them
  itself and announces itself by switching to the alt screen, so while one is up
  they go straight through to it. Nothing is captured during the alt screen
  either — a program that repaints its whole window on every keystroke would
  flush the shell history the buffer exists to hold. `\x1b[3J` (a remote
  explicitly erasing its scrollback) drops the history, `\x1b[2J` does not, so
  `clear` behaves here exactly as it does in your own terminal.

- **`Close all sessions` on the ssh sessions list** — a panel operation in the
  `Space` menu that ends every live session at once, asking first with the count
  in the question. It has **no letter, on purpose**: closing everything is
  destructive and rare, and a letter is what a hand finds by accident on a list
  it was only scrolling. The menu is the slow path — open it, walk to the row,
  press Enter — and slow is the correct speed for this one. (Every *letter*
  hotkey is still a menu row; the containment simply does not run the other way.)
- **`Alt+Enter` zooms a grid cell to fill the whole grid** — a screen of six
  terminals is what the grid is for, and it is also too cramped to work in one
  of them. `Enter` is "go in" all over sshu and a zoom is going further in,
  which is why `Alt+Esc` — the key that comes back out — is what leaves it, one
  layer at a time: the first press drops the zoom and keeps the keyboard in the
  cell, the next hands it back to the list. Steering with `Alt+arrows` stays
  zoomed, because the next terminal is usually one you want to read just as
  closely. With a single cell on the grid there is nothing to zoom, so the chord
  is not taken there — it goes to the remote like any other.

### Changed

- **Tabs are switched with `M` / `F` / `S`, not `Alt+p/f/s`** — the chord bought
  one specific thing: switching tab while a remote held the keyboard. That is no
  longer wanted — inside a pty every bare key belongs to the far end, and
  `Alt+Esc` comes out first, which is a move you were making anyway. What the
  chord cost was three of the app's most reachable keys, permanently. The `[Alt]`
  lead segment at the head of the tab strip goes with it: it was there to spell
  the other half of a chord, and there is no other half now.
- **The preference tab is now `[M]anage`** — it holds hosts, credentials and
  logs. Those are records, not settings, so the old name was answering for the
  wrong thing; renaming it also settles which letter it wants.
- **`[S]elect host` is now `[H]ost`** — `S` went to the SSH tab. The row was
  always "this side's **host**"; the verb moved into the hint, which is where a
  row explains itself.
- **`[P]rogress` is now `[J]obs`** — that window lists individual pieces of work
  and cancels them one at a time. It is not a progress bar, and `P` is free now
  either way.
- **`[D]uplicate` leaves the keyboard on the sessions list** — it used to open
  the second session and drop you straight into it. The Enter that ran it was an
  Enter on a *confirmation*, and a confirmation's Enter means "yes, do that" —
  the thing being confirmed was copying a connection, not entering it. On `[1]`,
  only Enter on a row hands the keyboard to a remote. Nothing is silent about
  it: the cursor lands on the new session, its cell appears on the grid wearing
  the cursor's echo, and the status slot counts one more. Connecting from the
  hosts table still lands in the remote — reaching a remote is what that key is
  for.
- **The ssh grid's cursor echo is no longer the focus blue** — walking `[1]`
  lights the matching cell on the grid, and it used to light it in exactly the
  colour that means "the keyboard is here". Two identical blue frames, one on
  the panel that has the keyboard and one on a cell that does not, is a question
  where there should have been an answer. The echo now wears the cursor's own
  colour — because that is what it is: the `[1]` cursor, drawn further away.
  The cell holding the keyboard is still the only blue frame on the grid.

### Fixed

- **`Ctrl+C` inside a session no longer quits sshu** — it is the far end's
  interrupt, and the most reflexive key a shell has. Reaching for it to kill a
  runaway command and losing every open session instead — without even the
  confirmation `q` asks for — was the app's most expensive misfire. The same
  applies inside `[e]dit`'s editor. It is still the emergency exit everywhere
  else, and nothing is stranded: `Alt+Esc` takes the keyboard back and `Ctrl+C`
  is itself again on the other side of it. External SIGINT / SIGTERM / SIGHUP
  are untouched and still leave no orphan.
- **The ssh sessions list scrolls to follow its cursor** — it never did. The
  viewport index was only ever clamped into range, so walking past the seventh
  or eighth session took the cursor off the bottom of the panel and left it
  there. `u` / `d` were wrong too, in the other direction: they were handed the
  panel's height in LINES as a page size measured in SESSIONS, so a half page
  moved a whole one. Rows are not a fixed height — a long address wraps and the
  `:port #N` tail takes a line of its own rather than being split — so both the
  page size and the scroll position are now counted against what each row
  actually draws.
- **The app log wraps to the full width of its panel** — it was breaking every
  line early to land on a separator, a rule written for the sessions list where
  hostnames read better broken after a dash. A remote's output is dense with
  exactly those characters — an IP is three dots, a path is a run of slashes —
  so every line lost up to a third of its width and the log read as text
  squeezed through a narrow channel. Two more things went with it: the message
  column had a floor that made every wrapped line wider than its own panel on
  anything under 24 columns (the outer clip then ate the words, not the
  layout), and the 15-column timestamp gutter now yields entirely on a panel too
  narrow to spare it rather than leaving four columns for the message.
- **Typing a filename no longer triggers hotkeys** — `V` opened the easter-egg
  splash and `?` opened the help while a search query was being typed, because
  each key carried its own list of exceptions and both lists missed the same
  case (the two `/` filters are panel state, not floats). There is now one
  question, asked in one place, and every bare global letter asks it.

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
