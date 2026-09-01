package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// minAppW is the narrowest terminal sshu draws in: the width of the short-label
// capsule strip. Below it the tab row cannot be honest about which tab is lit.
const minAppW = 20

// statusMinRoom is how much slack the tab row needs before it shows the status
// slot at all. Less than this and the text is all ellipsis, which reads as
// breakage rather than information.
const statusMinRoom = 6

// The tab row is ONE powerline strip, not three free-standing pills: a round cap
// opens it, the segments run together, slanted dividers separate them, a round
// cap closes it. Exactly one segment is lit.
//
// It was three separate capsules, and the unlit ones were filled with `crust` —
// a shade off the canvas — so they had no visible shape at all, while an
// unfocused PANEL chip (dark text on borderDim) was perfectly legible. The same
// app drew its two "not selected" states in opposite directions.
//
// Two things were tried before this. Raising the unlit fill to match the panel
// chip fixed visibility but made three filled pills in a row read as a strip of
// buttons — the thing the rule under this row exists to prevent. Outlining the
// unlit ones with thin half-circle caps (U+E0B7/E0B5) gave them a shape, but two
// thin arcs around dim text read as PARENTHESES rather than as a capsule.
//
// A chain answers both. It is one object, so the unlit segments are visibly part
// of something rather than floating; and it cannot be mistaken for a row of
// buttons, because a row of buttons has gaps and this has none. The earlier note
// here said sshu deliberately avoided filu's chain because "a chain says these
// are pages of one thing, and sshu's tabs are three co-existing surfaces" — but
// three co-existing surfaces of ONE app is exactly what the strip draws, and the
// separation that matters (chrome above, surface below) is the rule's job.
//
// The dividers slant "/", forward, in the direction the eye reads.

// tabChainW is the strip's width: two caps, a space either side of every label,
// and one divider between neighbours.
func tabChainW(labels []string) int {
	w := 2
	for i, l := range labels {
		if i > 0 {
			w++
		}
		w += dispW(l) + 2
	}
	return w
}

// tabChain renders the strip. The solid divider is drawn in the RIGHT segment's
// fill over the LEFT segment's, which is what makes the colour change land on
// the slant instead of on a cell boundary.
func tabChain(labels []string, active int) string {
	lit, unlit := focusColor, lipgloss.Color(surface0Hex)
	fill := func(i int) lipgloss.Color {
		if i == active {
			return lit
		}
		return unlit
	}

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(fill(0)).Render(capLeft))
	for i, lab := range labels {
		if i > 0 {
			prev, cur := fill(i-1), fill(i)
			st := lipgloss.NewStyle().Foreground(cur).Background(prev)
			div := slantSolid
			if prev == cur {
				// Same fill on both sides: a line, not a transition.
				st = lipgloss.NewStyle().Foreground(borderDim).Background(cur)
				div = slantThin
			}
			b.WriteString(st.Render(div))
		}
		seg := " " + lab + " "
		if i == active {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).
				Background(lit).Bold(true).Render(seg))
			continue
		}
		b.WriteString(lipgloss.NewStyle().Foreground(borderDim).Background(unlit).Render(seg))
	}
	b.WriteString(lipgloss.NewStyle().Foreground(fill(len(labels) - 1)).Render(capRight))
	return b.String()
}

// shortLabels drops each capsule to its "[N]" prefix. This is the narrow-width
// degradation for the tab row: the label is the content signal and losing it
// hurts, but a capsule strip wider than the terminal breaks the frame outright
// (§1.1 — narrow must stay usable).
func shortLabels(labels []string) []string {
	out := make([]string, len(labels))
	for i, l := range labels {
		if k := strings.Index(l, "]"); k >= 0 {
			out[i] = l[:k+1]
		} else {
			out[i] = l
		}
	}
	return out
}

// tabRow is the top content row: capsules on the left, a per-tab status slot
// right-aligned. This row replaces the panel border title entirely — the lit
// capsule is what says which surface you are on (§1.1).
//
// Always exactly one row of exactly w cells (§1.3): the status is truncated,
// the labels shorten, nothing wraps.
func tabRow(w int, labels []string, active int, status string) string {
	if tabChainW(labels)+1 > w {
		labels = shortLabels(labels)
	}

	var b strings.Builder
	b.WriteString(" ")
	b.WriteString(tabChain(labels, active))
	used := tabChainW(labels) + 1 // the leading indent
	if used >= w {
		return b.String() // pathological width; the guard in View catches this
	}

	// Right-align the status with one cell of breathing room at the edge; drop
	// it entirely rather than let it collide with the capsules.
	room := w - used - 1
	if s := truncate(status, room-1); status != "" && room >= statusMinRoom && s != "" {
		b.WriteString(strings.Repeat(" ", room-dispW(s)))
		b.WriteString(lipgloss.NewStyle().Foreground(dimColor).Render(s))
		b.WriteString(" ")
	} else {
		b.WriteString(strings.Repeat(" ", room+1))
	}
	return b.String()
}

// keyLegend renders the footer's "key desc" pairs. This is the mandatory
// disclosure channel for the two VTP entry keys (§A.1 / §A.2): a user who never
// read a README learns Space and ? exist by reading this row.
//
// When the terminal is too narrow, pairs are dropped from the RIGHT — the entry
// keys are listed first precisely so they are the last thing to go.
func keyLegend(pairs [][2]string, w int) string {
	const sep = "   "
	plainW := func(n int) int {
		total := 1
		for i := 0; i < n; i++ {
			if i > 0 {
				total += dispW(sep)
			}
			total += dispW(pairs[i][0]) + 1 + dispW(pairs[i][1])
		}
		return total
	}

	n := len(pairs)
	for n > 0 && plainW(n) > w {
		n--
	}

	k := lipgloss.NewStyle().Foreground(handColor)
	d := lipgloss.NewStyle().Foreground(dimColor)
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, k.Render(pairs[i][0])+" "+d.Render(pairs[i][1]))
	}
	return " " + strings.Join(parts, sep) + strings.Repeat(" ", max(0, w-plainW(n)))
}

// tabRule separates the tab row from the panels below it.
//
// It earns its row: without it the tab capsules and the panel capsules sit
// directly against each other and read as one strip of buttons. The line splits
// the screen into "which surface am I on" above and "the surface" below, which
// is what lets the panels wear capsules of their own without competing.
func tabRule(w int) string {
	return lipgloss.NewStyle().Foreground(borderDim).Render(strings.Repeat("─", max(0, w)))
}

func borderColor(focused bool) lipgloss.Color {
	if focused {
		return focusColor
	}
	return borderDim
}

// panelChip is a panel border title as one rounded capsule: round-left cap,
// dark-on-border-colour body, round-right cap.
//
// The capsules came off these titles once, on the grounds that a capsule reads
// as a button and a title is not one. The rule above the panels settles that:
// with the two zones visibly separated, a panel capsule is read as belonging to
// its panel rather than as another tab to press.
func panelChip(title string, focused bool) string {
	bc := borderColor(focused)
	cap := lipgloss.NewStyle().Foreground(bc)
	body := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(bc).Bold(true)
	return cap.Render(capLeft) + body.Render(title) + cap.Render(capRight)
}

// panelChrome frames body in a titled box whose border colour carries focus.
//
// Every line of body must already be innerW cells; short lines are padded, so a
// PTY frame arriving mid-resize cannot shear the border.
func panelChrome(innerW int, body []string, title string, focused bool) string {
	bc := borderColor(focused)
	bs := lipgloss.NewStyle().Foreground(bc)

	// An empty title means NO capsule. Rendering panelChip("") would still draw
	// both round caps with nothing between them — two stray glyphs sitting on
	// the border, which is what "no title" must not look like.
	chip, chipW := "", 0
	if title != "" {
		chip, chipW = panelChip(title, focused), dispW(title)+2
		if chipW > innerW {
			chip, chipW = "", 0
		}
	}

	out := make([]string, 0, len(body)+2)
	out = append(out, bs.Render("╭")+chip+
		bs.Render(strings.Repeat("─", max(0, innerW-chipW))+"╮"))
	side := bs.Render("│")
	for _, l := range body {
		out = append(out, side+l+strings.Repeat(" ", max(0, innerW-dispW(l)))+side)
	}
	out = append(out, bs.Render("╰"+strings.Repeat("─", innerW)+"╯"))
	return strings.Join(out, "\n")
}

// joinVertical / joinHorizontal are display-width aware block joins.
func joinVertical(blocks ...string) string {
	return lipgloss.JoinVertical(lipgloss.Left, blocks...)
}

func joinHorizontal(blocks ...string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, blocks...)
}
