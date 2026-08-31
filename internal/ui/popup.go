package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// popupLayerColor is the border colour for a popup at a given nesting depth
// (§2.2 / §6.3): brightness climbs with the stack, so the user can see which
// float is on top. Lavender is deliberately absent — that band belongs to user
// footprint and a popup border must never borrow it (§B).
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
// (§6.2). animFrames * animStep lands at ~128ms, inside the 100-200ms band
// where the motion registers without pacing the user.
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
// stops a keystroke landing on a half-drawn surface (§6.2).
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

// ------------------------------------------------------------------ drawing

// drawPopupBox is the shared popup frame (kbu form): the title sits in the top
// border, the hint in the bottom border, and the content is framed by one blank
// row top and bottom. One frame for every popup class means the user learns the
// shape once.
//
// The hint is not decoration — it is the standing disclosure of what this
// surface can do, and it is what lets a text-entry popup opt out of the Space
// entry key without opening a VTP hole (§4.5).
func drawPopupBox(bc lipgloss.Color, title, hint string, rows []string, innerW int) string {
	return drawPopupBoxPad(bc, title, hint, rows, innerW, true)
}

func drawPopupBoxPad(bc lipgloss.Color, title, hint string, rows []string, innerW int, pad bool) string {
	bs := lipgloss.NewStyle().Foreground(bc)
	ts := lipgloss.NewStyle().Foreground(bc).Bold(true)

	// A title or hint wider than the box would push the border out and shear the
	// frame — clip both. The hint arrives pre-styled from hintLegend, so it has to
	// be clipped ANSI-aware; only the title is styled here.
	title = truncate(title, innerW-1)
	hint = clipANSI(hint, innerW-1)

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

// capRows limits a popup to what the terminal can hold. A float taller than the
// canvas would push its own bottom border off screen and shear the frame, so the
// content is cut before the box is drawn rather than the box clipped after.
func capRows(rows []string, screenH int) []string {
	budget := max(1, screenH-6) // two borders, two padding rows, a margin
	if len(rows) > budget {
		return rows[:budget]
	}
	return rows
}

// popupInnerW picks a popup's inner width: what it asked for, capped so the box
// always leaves a margin inside the terminal.
func popupInnerW(screenW, want int) int {
	return max(10, min(want, screenW-6))
}

// hintLegend builds a popup's bottom-border hint: key bright, description dim.
// It is the same reading as the footer legend — bright is the key you press, dim
// is what it does — so the rule is learned once and holds everywhere (§4.4).
// Spacing is tighter than the footer's because a border line has no room to
// breathe: one space inside a pair, two between them.
func hintLegend(pairs [][2]string) string {
	k := lipgloss.NewStyle().Foreground(handColor)
	d := lipgloss.NewStyle().Foreground(dimColor)
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, k.Render(p[0])+" "+d.Render(p[1]))
	}
	return " " + strings.Join(parts, "  ") + " "
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
// [X]label (§4.4). The letter is shown uppercase for legibility; matchesHotkey
// is what keeps that honest. If the label already begins with the hotkey letter the
// bracket wraps it in place; otherwise it is prefixed. Core-key actions (Enter,
// Esc) never get brackets — their key goes in the hint column instead, so the
// bracket keeps meaning exactly one thing.
func bracketHotkey(label, key string) string {
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
