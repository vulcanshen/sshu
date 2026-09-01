package ui

import (
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vulcanshen/sshu/internal/version"
)

type splashTickMsg struct{}
type splashIdentityMsg struct{} // fires the name + tagline together
type splashHintMsg struct{}

// splashModel renders the sshu logo as a hidden easter egg (the `V` key), a
// sibling of kbu's and filu's splashes. The u-family mark is a navy U wrapping
// a gold figure that spells SSH — two S's stacked between the H's rails — and
// it reveals in that order: the background sheet, S, S, then the H, then the
// U frame rising around them.
type splashModel struct {
	active          bool
	pixelOrder      []int    // reveal order across all stages
	orderColor      []string // colour for pixelOrder[i] (parallel)
	stageEnds       []int    // cumulative pixel count at each stage's end
	stageStep       []int    // pixels revealed per tick within each stage
	beatsDone       int      // inter-stage holds already taken
	revealedCount   int
	identityVisible bool // "sshu" line
	versionVisible  bool // the version line
	taglineVisible  bool // the tagline line
	hintVisible     bool // the Esc hint
}

func newSplashModel() splashModel { return splashModel{} }

func (m splashModel) isActive() bool { return m.active }

// show activates the splash and returns the first animation tick. Reveal
// stages, each held apart by a beat, spell the name: (1) background — a dark
// sheet, row-major top-to-bottom sweep; (2) the top S, scattered in; (3) the
// bottom S; (4) the H — rails and crossbar, top to bottom; (5) the U frame
// (navy), bottom-to-top so it rises from the base around the mark. Then a
// hold reveals the name + version + tagline, and a final hold the Esc hint.
func (m *splashModel) show() tea.Cmd {
	m.active = true
	m.revealedCount = 0
	m.beatsDone = 0
	m.identityVisible = false
	m.versionVisible = false
	m.taglineVisible = false
	m.hintVisible = false

	rows, cols := len(logoPixels), len(logoPixels[0])
	// Background pass covers EVERY cell so the sheet fills solid; the later
	// passes paint over it (overwrite, not gaps).
	var bg []int
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			bg = append(bg, r*cols+c)
		}
	}
	// band returns one gold band's pixels in the order given: top-to-bottom,
	// shuffled, or bottom-to-top.
	band := func(b byte, order string) []int {
		var px []int
		for r := 0; r < rows; r++ {
			for c := 0; c < cols; c++ {
				if logoPixels[r][c] == b {
					px = append(px, r*cols+c)
				}
			}
		}
		switch order {
		case "shuffle":
			rand.Shuffle(len(px), func(i, j int) { px[i], px[j] = px[j], px[i] })
		case "rise":
			for i, j := 0, len(px)-1; i < j; i, j = i+1, j-1 {
				px[i], px[j] = px[j], px[i]
			}
		}
		return px
	}

	m.pixelOrder, m.orderColor, m.stageEnds, m.stageStep = nil, nil, nil, nil
	addStage := func(px []int, color string, step int) {
		m.pixelOrder = append(m.pixelOrder, px...)
		for range px {
			m.orderColor = append(m.orderColor, color)
		}
		m.stageEnds = append(m.stageEnds, len(m.pixelOrder))
		m.stageStep = append(m.stageStep, step)
	}
	addStage(bg, logoBg, cols) // one full row per tick
	addStage(band('S', "shuffle"), logoGold, 2)
	addStage(band('T', "shuffle"), logoGold, 2)
	addStage(band('H', "down"), logoGold, 3)
	addStage(band('U', "rise"), logoNavy, 3) // the frame rises from the base

	return tea.Tick(10*time.Millisecond, func(time.Time) tea.Msg { return splashTickMsg{} })
}

// sshu logo — generated from docs/icon.svg by cell-center sampling (25×25,
// trimmed to 21 rows; the icon carries a few 10×15 rects, which is why the
// sampling is by center rather than by a fixed 10px grid). D = background
// sheet, U = navy frame; the gold mark spells SSH — S/T are the two S's,
// H the rails + crossbar.
var logoPixels = [21]string{
	"DDDDDDDDDDDDDDDDDDDDDDDDD",
	"DDUUUDHHDSSSSSSSDHHDUUUDD",
	"DDDUUDHHDSDDDDDDDHHDUUDDD",
	"DDDUUDHHDSSSSSSSDHHDUUDDD",
	"DDDUUDHHDDDDDDDDDHHDUUDDD",
	"DDDUUDHHDDDDDDDSDHHDUUDDD",
	"DDDUUDHHDSSSSSSSDHHDUUDDD",
	"DDDUUDHHDDDDDDDDDHHDUUDDD",
	"DDDUUDHHHHHHHHHHHHHDUUDDD",
	"DDDUUDHHHHHHHHHHHHHDUUDDD",
	"DDDUUDHHDDDDDDDDDHHDUUDDD",
	"DDDUUDHHDTTTTTTTDHHDUUDDD",
	"DDDUUDHHDTDDDDDDDHHDUUDDD",
	"DDDUUDHHDTTTTTTTDHHDUUDDD",
	"DDDUUDHHDDDDDDDDDHHDUUDDD",
	"DDDUUDHHDDDDDDDTDHHDUUDDD",
	"DDDUUDHHDTTTTTTTDHHDUUDDD",
	"DDDUUDDDDDDDDDDDDDDDUUDDD",
	"DDUUUUUUUUUUUUUUUUUUUUUDD",
	"DUUUUUUUUUUUUUUUUUUUUUUUD",
	"DDDDDDDDDDDDDDDDDDDDDDDDD",
}

const (
	logoBg   = "#313244" // background sheet (catppuccin surface0)
	logoNavy = "#205090" // U frame (icon.svg)
	logoGold = "#f2b753" // the gold mark (icon.svg)

	// pixelGlyph is one logo pixel: the nf-fa-square Nerd Font glyph, coloured
	// per cell (same glyph everywhere, matching the kbu/filu splashes).
	pixelGlyph = "\uf0c8"
)

func (m splashModel) render(width, height int) string {
	if !m.active {
		return ""
	}

	// Colour each revealed cell by the stage that painted it (orderColor is
	// parallel to pixelOrder). The background pass covers every cell; the later
	// passes come after it in pixelOrder, so they overwrite where they land.
	cols := len(logoPixels[0])
	cellColor := make([]string, len(logoPixels)*cols)
	for i := 0; i < m.revealedCount; i++ {
		cellColor[m.pixelOrder[i]] = m.orderColor[i]
	}

	// A pixel is the glyph in the cell's colour plus a space (two cells per
	// pixel); unrevealed cells are two blanks.
	var logoLines []string
	for r := 0; r < len(logoPixels); r++ {
		var line strings.Builder
		for c := 0; c < cols; c++ {
			if color := cellColor[r*cols+c]; color != "" {
				line.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(pixelGlyph + " "))
			} else {
				line.WriteString("  ")
			}
		}
		logoLines = append(logoLines, line.String())
	}
	logo := strings.Join(logoLines, "\n")

	// Caption space is always reserved so the logo doesn't shift when text
	// appears.
	logoW := cols * 2
	name := lipgloss.NewStyle().Foreground(focusColor).Bold(true)
	line := lipgloss.NewStyle().Foreground(focusColor)
	dim := lipgloss.NewStyle().Foreground(dimColor)
	identityText, versionText, taglineText, hintText := " ", " ", " ", " "
	if m.identityVisible {
		identityText = name.Render("sshu")
	}
	if m.versionVisible {
		versionText = line.Render(version.Display())
	}
	if m.taglineVisible {
		taglineText = line.Render("A terminal front end for ssh and sftp")
	}
	if m.hintVisible {
		hintText = dim.Render("Press Esc to close")
	}
	caption := "\n\n" +
		lipgloss.PlaceHorizontal(logoW, lipgloss.Center, identityText) +
		"\n" +
		lipgloss.PlaceHorizontal(logoW, lipgloss.Center, versionText) +
		"\n" +
		lipgloss.PlaceHorizontal(logoW, lipgloss.Center, taglineText) +
		"\n\n" +
		lipgloss.PlaceHorizontal(logoW, lipgloss.Center, hintText)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, logo+caption)
}

// update handles key events and animation ticks while the splash is active.
func (m splashModel) update(msg tea.Msg) (splashModel, tea.Cmd) {
	if !m.active {
		return m, nil
	}
	switch msg.(type) {
	case tea.KeyMsg:
		// Any key dismisses the easter egg.
		m = splashModel{}
	case splashTickMsg:
		// Hold once at each stage boundary before the next begins. Stages
		// reveal in order, so beatsDone is also the next boundary to check.
		if m.beatsDone < len(m.stageEnds)-1 && m.revealedCount == m.stageEnds[m.beatsDone] {
			m.beatsDone++
			return m, tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg { return splashTickMsg{} })
		}
		if m.revealedCount < len(m.pixelOrder) {
			// Advance by the current stage's step, clamped to that stage's end
			// so the boundary beats fire cleanly.
			stage := 0
			for stage < len(m.stageEnds)-1 && m.revealedCount >= m.stageEnds[stage] {
				stage++
			}
			newCount := m.revealedCount + m.stageStep[stage]
			if newCount > m.stageEnds[stage] {
				newCount = m.stageEnds[stage]
			}
			m.revealedCount = newCount
			return m, tea.Tick(10*time.Millisecond, func(time.Time) tea.Msg { return splashTickMsg{} })
		}
		// Pixels done — reveal the name + tagline after a brief hold.
		if !m.identityVisible {
			return m, tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg { return splashIdentityMsg{} })
		}
	case splashIdentityMsg:
		m.identityVisible = true
		m.versionVisible = true
		m.taglineVisible = true
		return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return splashHintMsg{} })
	case splashHintMsg:
		m.hintVisible = true
	}
	return m, nil
}
