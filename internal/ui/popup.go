package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// popupLayerColor is the border colour for a popup at a given nesting depth
// (tdp D2): brightness climbs with the stack, so the user can see which
// float is on top. Lavender is deliberately absent — that band belongs to user
// footprint and a popup border must never borrow it (tdp P4).
func popupLayerColor(layer int) lipgloss.Color {
	switch {
	case layer <= 1:
		return lipgloss.Color("#A4C0FA")
	case layer == 2:
		return lipgloss.Color("#94C3F5")
	case layer == 3:
		return lipgloss.Color("#84C5F0")
	default:
		return lipgloss.Color("#74c7ec")
	}
}

// ---------------------------------------------------------------- animation

// A popup animates in and out, or the user cannot feel the z-axis change
// (tdp F2). animFrames * animStep lands at ~128ms, the family default (tdp D3):
// the motion registers without pacing the user.
const (
	animFrames = 8
	animStep   = 16 * time.Millisecond
)

type animPhase int

const (
	animClosed animPhase = iota
	animOpening
	animOpen
	animClosing
)

// AnimTickMsg drives one animator. Target names the animator: every popup owns
// a distinct name so two open at once cannot consume each other's ticks.
type AnimTickMsg struct{ Target string }

type popupAnimator struct {
	target string
	phase  animPhase
	frame  int
}

func newPopupAnimator(target string) popupAnimator {
	return popupAnimator{target: target}
}

func (a popupAnimator) isActive() bool { return a.phase != animClosed }

// owns reports whether this float still owns the keyboard.
//
// A CLOSING popup does not. The action is committed and the user is back on the
// panel — the animation is a visual, not a modal state. Keeping the keyboard
// until it finishes makes the first keystroke after ANY menu commit disappear,
// which reads as "the app dropped that key", not as "the popup was still busy".
//
// An OPENING one does own it, and deliberately swallows: isInteractive is what
// stops a keystroke landing on a half-drawn surface (tdp F2).
func (a popupAnimator) owns() bool {
	return a.phase == animOpening || a.phase == animOpen
}

// isInteractive gates key handling: a popup mid-animation is on screen but not
// yet listening, so a keystroke cannot land on a half-drawn surface.
func (a popupAnimator) isInteractive() bool { return a.phase == animOpen }

func (a popupAnimator) progress() float64 {
	switch a.phase {
	case animOpen:
		return 1
	case animClosed:
		return 0
	}
	return float64(a.frame) / animFrames
}

func (a *popupAnimator) open() tea.Cmd {
	a.phase, a.frame = animOpening, 0
	return a.tickCmd()
}

func (a *popupAnimator) close() tea.Cmd {
	if a.phase == animClosed {
		return nil
	}
	a.phase, a.frame = animClosing, animFrames
	return a.tickCmd()
}

func (a *popupAnimator) tick(msg AnimTickMsg) tea.Cmd {
	if msg.Target != a.target {
		return nil
	}
	switch a.phase {
	case animOpening:
		if a.frame++; a.frame >= animFrames {
			a.phase = animOpen
			return nil
		}
	case animClosing:
		if a.frame--; a.frame <= 0 {
			a.phase = animClosed
			return nil
		}
	default:
		return nil
	}
	return a.tickCmd()
}

func (a popupAnimator) tickCmd() tea.Cmd {
	t := a.target
	return tea.Tick(animStep, func(time.Time) tea.Msg { return AnimTickMsg{Target: t} })
}

// animRows is how every popup animates without writing its own animation code:
// the box is drawn with a growing slice of its content rows, so it expands from
// a title bar into the full box (and back). Because the overlay centres the
// result, it reads as growing from the middle.
func animRows(a popupAnimator, rows []string) []string {
	p := a.progress()
	if p >= 1 {
		return rows
	}
	n := int(float64(len(rows))*p + 0.5)
	return rows[:min(max(0, n), len(rows))]
}

// ------------------------------------------------------------------ loading

// A popup whose content is still on its way says so after its title with a
// turning icon (tdp F7, D3): the remote [v]iew being read, [e]dit fetching or
// writing back, the known_hosts fetch waiting for a key. The icon is webu's —
// Nerd Font's circle slices, one square cell filling round — read out of the
// installed font's cmap (U+F0A9E–U+F0AA5), never remembered.
var loadingFrames = []string{
	string(rune(0xf0a9e)), string(rune(0xf0a9f)), string(rune(0xf0aa0)), string(rune(0xf0aa1)),
	string(rune(0xf0aa2)), string(rune(0xf0aa3)), string(rune(0xf0aa4)), string(rune(0xf0aa5)),
}

// loadingStep is how long one frame lasts: a turn every 720ms.
const loadingStep = 90 * time.Millisecond

// loadingNow is the clock the icon reads — a seam, so a test can turn it.
var loadingNow = time.Now

// loadingIcon is the frame due now. It is read from the clock rather than
// counted, so the icon is right however many redraws land: a stray tick costs
// a redraw, never a jump.
func loadingIcon() string {
	n := loadingNow().UnixNano() / int64(loadingStep)
	return loadingFrames[n%int64(len(loadingFrames))]
}

// loadingTitle puts the icon after a title that is loading. The title sits in
// the box's top border, so the icon takes the layer colour, bold, with it.
func loadingTitle(title string, loading bool) string {
	if !loading {
		return title
	}
	return title + loadingIcon() + " "
}

// loadingTickMsg repaints a loading icon. It is only re-armed while something
// is loading: an idle sshu must not repaint for an icon nobody can see.
type loadingTickMsg struct{}

func loadingTick() tea.Cmd {
	return tea.Tick(loadingStep, func(time.Time) tea.Msg { return loadingTickMsg{} })
}

// ------------------------------------------------------------------ drawing

// drawPopupBox is the shared popup frame (kbu form): the title sits in the top
// border, the hint in the bottom border, and the content is framed by one blank
// row top and bottom. One frame for every popup class means the user learns the
// shape once.
//
// The hint is not decoration — it is the standing disclosure of what this
// surface can do, and it is what lets a text-entry popup opt out of the Space
// entry key without opening a hole in tdp K8.
func drawPopupBox(bc lipgloss.Color, title string, pairs [][2]string, rows []string, innerW int) string {
	return drawPopupBoxPad(bc, title, pairs, rows, innerW, true)
}

func drawPopupBoxPad(bc lipgloss.Color, title string, pairs [][2]string, rows []string, innerW int, pad bool) string {
	bs := lipgloss.NewStyle().Foreground(bc)
	ts := lipgloss.NewStyle().Foreground(bc).Bold(true)

	// A title or hint wider than the box would push the border out and shear the
	// frame. The title is cut; the hint gives up whole items from the end, as the
	// footer does, never one cut in the middle (tdp D3).
	title = truncate(title, innerW-1)
	hint := fitLegend(pairs, innerW-1)

	var b strings.Builder
	b.WriteString(bs.Render("╭─") + ts.Render(title) +
		bs.Render(strings.Repeat("─", max(0, innerW-1-dispW(title)))+"╮") + "\n")

	left, right := bs.Render("│"), bs.Render("│")
	padRow := left + strings.Repeat(" ", innerW) + right + "\n"
	if pad {
		b.WriteString(padRow)
	}
	for _, line := range rows {
		line = clipANSI(line, innerW)
		b.WriteString(left + line + strings.Repeat(" ", max(0, innerW-dispW(line))) + right + "\n")
	}
	if pad {
		b.WriteString(padRow)
	}
	// The hint is rendered verbatim: hintLegend already coloured it, and
	// re-styling would flatten the key/description distinction back out.
	b.WriteString(bs.Render("╰─") + hint +
		bs.Render(strings.Repeat("─", max(0, innerW-1-dispW(hint)))+"╯"))
	return b.String()
}

// popupBudget is how many content rows a padded popup can hold: two borders,
// two padding rows and a margin top and bottom (tdp F7). A popup whose content
// is longer scrolls inside the box; capRows is only the last guard.
func popupBudget(screenH int) int { return max(1, screenH-6) }

// capRows limits a popup to what the terminal can hold. A float taller than the
// canvas would push its own bottom border off screen and shear the frame, so the
// content is cut before the box is drawn rather than the box clipped after.
// Every popup that can outgrow the screen scrolls first (tdp F7); this is what
// stops a resize from shearing the frame before the popup catches up.
func capRows(rows []string, screenH int) []string {
	if budget := popupBudget(screenH); len(rows) > budget {
		return rows[:budget]
	}
	return rows
}

// popupMaxW is the widest a popup gets (tdp F7): on a wide terminal a menu's
// names and descriptions would otherwise sit a screen apart.
const popupMaxW = 120

// popupInnerW is every popup's inner width (tdp F7): the box is
// min(terminal width − 2, 120), one column of margin each side, and the two
// border columns come out of that. One width for every popup, whatever it
// holds, so no box is a surprise and none changes width while it is open —
// content that does not fit wraps or is cut, the box does not grow for it
// (tdp D4).
func popupInnerW(screenW int) int {
	return max(1, min(screenW-2, popupMaxW)-2)
}

// terminalInnerW is the terminal class's inner width (tdp F7): a child process
// needs the room, so it takes the whole terminal less one column each side,
// with no 120 cap.
func terminalInnerW(screenW int) int { return max(1, screenW-4) }

// fillRows pads rows with blank lines to n, so a popup keeps the height it
// opened with when its content comes up short (tdp F7).
func fillRows(rows []string, n, innerW int) []string {
	for len(rows) < n {
		rows = append(rows, spaces(innerW))
	}
	return rows
}

// scrollTop keeps row cursor inside a window of vis rows over n, moving the
// window only as far as it has to.
func scrollTop(top, cursor, vis, n int) int {
	if vis <= 0 || n <= vis {
		return 0
	}
	if cursor < top {
		top = cursor
	}
	if cursor >= top+vis {
		top = cursor - vis + 1
	}
	return clamp(top, 0, n-vis)
}

// hintLegend builds a popup's bottom-border hint: key bright, description dim.
// It is the same reading as the footer legend — bright is the key you press, dim
// is what it does — so the rule is learned once and holds everywhere (tdp M5).
// Spacing is tighter than the footer's because a border line has no room to
// breathe: one space inside a pair, two between them.
func hintLegend(pairs [][2]string) string {
	// The same blue as the footer, and for the same reason: these two ARE the
	// one legend at two scales, so a key that is blue on the app's bottom row
	// cannot be a different colour on a popup's (tdp M5).
	//
	// Written key:description, one space between items (tdp M5): the colour
	// tells the key from its word, so the colon goes with the word, dim. A pair
	// with no key is not a key at all — a position such as "3 of 12" — and is
	// drawn dim, with no colon.
	k := lipgloss.NewStyle().Foreground(focusColor)
	d := lipgloss.NewStyle().Foreground(dimColor)
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, legendPair(k, d, p))
	}
	return " " + strings.Join(parts, " ") + " "
}

// fitLegend is hintLegend within w cells: whole items are dropped from the
// end until the rest fits (tdp D3). Only when not even the first one fits is it
// cut — a box that narrow has no room for any key, and the frame still must
// not shear.
func fitLegend(pairs [][2]string, w int) string {
	n := len(pairs)
	for n > 1 && legendW(pairs[:n]) > w {
		n--
	}
	return clipANSI(hintLegend(pairs[:n]), w)
}

// legendW is how wide hintLegend draws pairs.
func legendW(pairs [][2]string) int {
	w := 2 + max(0, len(pairs)-1)
	for _, p := range pairs {
		w += legendPairW(p)
	}
	return w
}

// legendPair draws one item of a hint or the footer.
func legendPair(k, d lipgloss.Style, p [2]string) string {
	if p[0] == "" {
		return d.Render(p[1])
	}
	return k.Render(p[0]) + d.Render(":"+p[1])
}

// legendPairW is how wide legendPair draws p.
func legendPairW(p [2]string) int {
	if p[0] == "" {
		return dispW(p[1])
	}
	return dispW(p[0]) + 1 + dispW(p[1])
}

// sameHotkey reports whether two declared keys are the same letter, ignoring
// case. Not a dispatch rule — dispatch is exact — but two actions a case apart
// have nothing but the bracket to tell them apart, so the collision test uses it.
func sameHotkey(a, b string) bool {
	if len(a) != 1 || len(b) != 1 {
		return a == b
	}
	return strings.EqualFold(a, b)
}

// hotkeyIndex picks which of keys a keystroke fires, or -1.
//
// EXACTLY the declared key, case and all. The bracket is generated from the same
// string (bracketHotkey), so what is on screen is the whole binding — nothing
// fires that the marking does not name, and nothing the marking names fails.
//
// This used to fall back to a case-insensitive match, which dated from when the
// tables said `c` and the display uppercased it to [C]: pressing C hit nothing,
// so matching stopped caring about case. Printing the key as declared fixed that
// at its source, and the fallback survived as an invisible second binding — one
// that fires [C]lose on a bare `c`, and that would fire [C]lear marks in tab [2]
// where lower case is supposed to mean the row.
//
// Its removal is also what lets `t`/`T` and `x`/`X` mean two different things
// without a special case, and what makes navigation's letters safe without a
// guard here: nothing folds onto `d` any more, because nothing folds at all.
// That no action DECLARES a navigation key is checked by
// TestNoActionClaimsANavigationKey.
func hotkeyIndex(keys []string, pressed string) int {
	for i, k := range keys {
		if k == pressed {
			return i
		}
	}
	return -1
}

// bracketHotkey marks a letter hotkey the one way the whole app marks them:
// [X]label (tdp M5). The letter is shown uppercase for legibility; matchesHotkey
// is what keeps that honest. If the label already begins with the hotkey letter the
// bracket wraps it in place; otherwise it is prefixed. Core-key actions (Enter,
// Esc) never get brackets — their key goes in the hint column instead, so the
// bracket keeps meaning exactly one thing.
func bracketHotkey(label, key string) string {
	// A core key is written into the label, in front (tdp M5, D4: [Enter] Edit).
	if key == "enter" {
		return "[Enter] " + label
	}
	if len(key) != 1 {
		return label
	}
	// The key is shown EXACTLY as declared, and it is also the only key that
	// fires (hotkeyIndex). Where two actions share a letter in different cases,
	// the bracket is the only thing telling them apart on screen.
	if label != "" && strings.EqualFold(label[:1], key) {
		return "[" + key + "]" + label[1:]
	}
	return "[" + key + "] " + label
}
