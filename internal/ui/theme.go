package ui

import "github.com/charmbracelet/lipgloss"

// Colour anchors (catppuccin-mocha) — docs/sshu-ui-design.md §2.1 / §B.
// Assigned once, derived everywhere. Each tier is reserved: nothing borrows
// another's band, or the user has to learn which meaning a colour carries where.
var (
	// structural (system) — panel chrome, the active tab capsule, and the KEY
	// half of every legend (footer and popup hint alike, §4.4). Never user
	// state: a key name is the app naming its own controls, which is what puts
	// it in this band rather than in the cursor's.
	focusColor = lipgloss.Color("#89b4fa") // blue
	borderDim  = lipgloss.Color("#585b70") // surface2: unselected card border
	// cursor — "the current hand".
	handColor = lipgloss.Color("#bac2de") // subtext1
	// the form row under edit — label, value and caret alike, so the row reads as
	// one thing. Distinct from handColor on purpose: handColor is the cursor on a
	// list row you would ACT on, lavender is the field you are CHANGING.
	//
	// This spends the band that was pencilled in for "user footprint" (§B), so
	// pins / recent connections will need a colour of their own.
	editColor = lipgloss.Color("#b4befe") // lavender
	// neutral text.
	textColor = lipgloss.Color("#cdd6f4") // text
	dimColor  = lipgloss.Color("#6c7086") // overlay0: glyphs, hints, secondary
	// override — jumps out of the brightness hierarchy entirely and never takes
	// part in the popup layer scale (§2.4). Reserved for warning and error, which
	// is why auth method is encoded by glyph and not by colour.
	warnColor = lipgloss.Color("#f38ba8") // red
	// the session currently drawn in panel [5]. A FOREGROUND signal on purpose:
	// the list cursor already owns the background, and one row can only carry one
	// background — putting both there would make them fight for the same channel.
	liveColor = lipgloss.Color("#a6e3a1") // green
	// selection mode: a pty cell that has stopped following the remote so its
	// text can be swept and copied (§11.33). A band of its own because it is
	// neither focus nor warning — the panel still has the keyboard, and nothing
	// is wrong; it is doing something ELSE with the keyboard, and that is the
	// one thing the frame has to be able to say at a glance.
	selectColor = lipgloss.Color("#f9e2af") // yellow
	// the selected row in the hosts table. Blue rather than handColor because
	// subtext1 sits too close to textColor to read as a highlight.
	//
	// It shares the structural band with panel chrome (§B). The two are the same
	// idea at different scales — "this is where you are" for a panel and for a
	// row — and they never touch: chrome is the frame, this is inside it. List
	// cursors elsewhere are still handColor bars; if that split starts to grate,
	// the fix is to move THEM to blue, not to move this back.
	rowSelColor = focusColor
)

const (
	baseHex  = "#1e1e2e" // canvas; also dark text on a bright chip
	crustHex = "#11111b" // recessed background of an inactive capsule
)

// Nerd Font glyphs. Never a PUA literal in source — build them from the rune so
// the codepoint stays greppable and the file stays editor-safe (kbu / filu rule).
//
// The auth glyph is the one that varies with its value (key vs lock), which is
// how auth method is encoded — deliberately NOT by colour, since peach/red are
// reserved for warning/error and a peach "password" would read as "this host is
// broken" (§B).
var (
	capLeft  = string(rune(0xe0b6)) // powerline round-left  — capsule start
	capRight = string(rune(0xe0b4)) // powerline round-right — capsule end
	// The tab strip's inner dividers. The HARD one is a filled triangle and
	// carries a colour change (lit to unlit); the SOFT one is the same shape as
	// an outline, and only draws a line where the fill is the same on both sides.
	// Both point right, which is the direction the strip is read in.
	dividerHard = string(rune(0xe0b0)) // pl-left_hard_divider
	dividerSoft = string(rune(0xe0b1)) // pl-left_soft_divider

	// The auth column is the one place a glyph still carries meaning in the hosts
	// table: it IS the type distinction. The other fields lost theirs when the
	// cards became rows — a column header names them now, which a card could not.
	glyphKey  = string(rune(0xf084))  // nf-fa-key  — privatekey
	glyphLock = string(rune(0xf023))  // nf-fa-lock — password
	glyphCred = string(rune(0xf05d2)) // nf-md-card_account_details — credential

	// Popup title glyphs — the type signal half of a surface label (§3.4).
	glyphMenu    = string(rune(0xf0c9)) // nf-fa-bars            — Space menu
	glyphHelp    = string(rune(0xf059)) // nf-fa-question_circle — help
	glyphWarn    = string(rune(0xf071)) // nf-fa-warning         — confirm
	glyphPlus    = string(rune(0xf067)) // nf-fa-plus            — create form
	glyphPencil  = string(rune(0xf040)) // nf-fa-pencil          — edit form
	glyphConnect = string(rune(0xf120)) // nf-fa-terminal        — connect
	glyphInfo    = string(rune(0xf05a)) // nf-fa-info_circle     — toast
	glyphSearch  = string(rune(0xf002)) // nf-fa-search          — file picker

	// The Auth field's radio buttons. `radiobox_blank` is an MDI alias — the
	// font exposes that codepoint under the name checkbox-blank-circle-outline —
	// so anyone checking the cmap will find the other name and should not
	// "correct" it.
	glyphRadioOff = string(rune(0xf043d)) // nf-md-radiobox_blank
	glyphRadioOn  = string(rune(0xf043e)) // nf-md-radiobox_marked

	// tab [2] sftp
	glyphUpload = string(rune(0xf1065)) // nf-md-transfer
	glyphDir    = string(rune(0xe5ff))  // nf-custom-folder
	glyphFile   = string(rune(0xf15b))  // nf-fa-file
	glyphMark   = string(rune(0xf0e1e)) // nf-md-check_bold — marked for transfer
	// The viewer wore glyphSearch until it was seen next to an actual search:
	// a magnifier is the symbol `/` owns, and reusing it made the two surfaces
	// claim the same meaning (§3).
	glyphEye = string(rune(0xf06d0)) // nf-md-eye_outline — [v]iew
	// The ssh list's display column: a session with a cell on the grid wears
	// the monitor, one without wears the struck-through monitor. Two SHAPES,
	// not one shape in two colours, so the state survives any palette.
	glyphMonitor    = string(rune(0xf0379)) // nf-md-monitor
	glyphMonitorOff = string(rune(0xf0d90)) // nf-md-monitor_off
	glyphGrid       = string(rune(0xf0570)) // nf-md-view_grid — the custom-grid prompt
	// glyphHistory marks a cell showing scrollback rather than live output. An
	// up arrow was the obvious pick and the wrong one: it says which DIRECTION
	// the view moved, when the thing that has to be unmistakable is WHAT is on
	// screen — the past.
	glyphHistory = string(rune(0xf02da)) // nf-md-history
	// arrowGlyphs labels the left/right keys in a hint. Plain Unicode arrows, not
	// Nerd Font: these sit in a border line where a mis-measured glyph shears the
	// frame, and U+2190/2192 are single-width everywhere.
	arrowGlyphs = "\u2190\u2192"
	arrowUpDown = "\u2191\u2193"
)
