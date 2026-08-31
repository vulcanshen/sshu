package ui

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// filePicker is the menu class with a filter row (filu's finder form): type to
// narrow, arrows to select, Enter to take. It exists so an identity file is
// PICKED rather than typed — a mistyped key path fails at connect time, far from
// where the mistake was made.
//
// It is not modal. Letters always filter and the arrows always move, so there is
// no "input state" versus "list state" to learn — the same split the form makes
// (§4.5): in a text-entry surface, letters type and arrows navigate.
type filePicker struct {
	anim    popupAnimator
	root    string
	entries []pickerEntry
	matches []int // indices into entries, best first
	query   string
	cursor  int
	top     int
	note    string // why the list is empty or short — never a silent cap
	layer   int
	screenW int
	screenH int
}

type pickerEntry struct {
	path  string // absolute
	label string // relative to root — what the user reads
	mode  fs.FileMode
	size  int64
}

// pickerCap bounds the walk. ~/.ssh is tiny, so this only ever fires on a
// pathological directory; when it does, the note says so rather than quietly
// showing a truncated list.
const pickerCap = 2000

func newFilePicker() filePicker { return filePicker{anim: newPopupAnimator("picker")} }

func (m filePicker) isActive() bool      { return m.anim.isActive() }
func (m filePicker) isInteractive() bool { return m.anim.isInteractive() }
func (m *filePicker) close() tea.Cmd     { return m.anim.close() }
func (m *filePicker) setSize(w, h int)   { m.screenW, m.screenH = w, h }

// identityRoot is where ssh keys live. The picker deliberately does NOT fall
// back to walking $HOME: a recursive walk of a home directory would hang the UI,
// and the field stays typeable for the rare key that lives elsewhere.
//
// identityRootOverride is a test seam, mirroring store's configPathOverride.
var identityRootOverride string

func identityRoot() string {
	if identityRootOverride != "" {
		return identityRootOverride
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh")
}

func (m *filePicker) open(root string, layer int) tea.Cmd {
	m.root, m.layer = root, layer
	m.query, m.cursor, m.top, m.note = "", 0, 0, ""
	m.entries = nil

	if root == "" {
		m.note = "cannot locate your home directory"
	} else if st, err := os.Stat(root); err != nil || !st.IsDir() {
		// Short on purpose: the title already names the directory, and a note long
		// enough to be clipped would lose its advice off the right edge.
		m.note = "directory not found — type the path instead"
	} else {
		m.entries, m.note = scanFiles(root)
	}
	m.refilter()
	return m.anim.open()
}

// scanFiles walks root for regular files. Unreadable subtrees are skipped rather
// than aborting the walk — one bad directory should not empty the picker.
func scanFiles(root string) ([]pickerEntry, string) {
	var out []pickerEntry
	capped := false
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		if len(out) >= pickerCap {
			capped = true
			return filepath.SkipAll
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			rel = p
		}
		out = append(out, pickerEntry{path: p, label: rel, mode: info.Mode(), size: info.Size()})
		return nil
	})
	if capped {
		return out, fmt.Sprintf("stopped at %d files", pickerCap)
	}
	if len(out) == 0 {
		return out, "no files here"
	}
	return out, ""
}

func (m *filePicker) refilter() {
	type scored struct{ idx, score int }
	var hits []scored
	for i, e := range m.entries {
		if s, ok := fuzzyScore(e.label, m.query); ok {
			hits = append(hits, scored{i, s})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool {
		if hits[a].score != hits[b].score {
			return hits[a].score > hits[b].score
		}
		return m.entries[hits[a].idx].label < m.entries[hits[b].idx].label
	})
	m.matches = m.matches[:0]
	for _, h := range hits {
		m.matches = append(m.matches, h.idx)
	}
	m.cursor, m.top = 0, 0
}

// fuzzyScore matches q as a case-insensitive subsequence of s. Runs of adjacent
// characters and matches at a word boundary score high, so typing "ided" puts
// id_ed25519 above something that merely happens to contain those letters.
func fuzzyScore(s, q string) (int, bool) {
	if q == "" {
		return 0, true
	}
	rs := []rune(strings.ToLower(s))
	rq := []rune(strings.ToLower(q))

	score, at, prev := 0, 0, -2
	for _, want := range rq {
		found := -1
		for i := at; i < len(rs); i++ {
			if rs[i] == want {
				found = i
				break
			}
		}
		if found < 0 {
			return 0, false
		}
		if found == prev+1 {
			score += 8 // adjacent to the previous hit
		} else {
			score++
		}
		if found == 0 || strings.ContainsRune("/_-.", rs[found-1]) {
			score += 4 // start of a path or word segment
		}
		prev, at = found, found+1
	}
	return score - len(rs)/8, true // shorter paths win ties
}

func (m *filePicker) update(msg tea.KeyMsg) (picked string, done bool) {
	if !m.anim.isInteractive() {
		return "", false
	}
	switch msg.Type {
	case tea.KeyUp:
		m.cursor = moveCursor(m.cursor, len(m.matches), "k", m.visible())
	case tea.KeyDown:
		m.cursor = moveCursor(m.cursor, len(m.matches), "j", m.visible())
	case tea.KeyEnter:
		if m.cursor < len(m.matches) {
			return store.FoldHome(m.entries[m.matches[m.cursor]].path), true
		}
	case tea.KeyBackspace:
		if r := []rune(m.query); len(r) > 0 {
			m.query = string(r[:len(r)-1])
			m.refilter()
		}
	case tea.KeySpace:
		m.query += " "
		m.refilter()
	case tea.KeyRunes:
		m.query += string(msg.Runes)
		m.refilter()
	}
	m.scroll()
	return "", false
}

func (m *filePicker) scroll() {
	vis := m.visible()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+vis {
		m.top = m.cursor - vis + 1
	}
	m.top = max(0, m.top)
}

// visible is how many result rows fit: the box costs its borders, and the query
// row and its divider come out of the content budget.
func (m filePicker) visible() int { return max(1, m.screenH-9) }

func (m filePicker) view() string {
	innerW := popupInnerW(m.screenW, 54)

	dim := lipgloss.NewStyle().Foreground(dimColor)
	txt := lipgloss.NewStyle().Foreground(textColor)
	hand := lipgloss.NewStyle().Foreground(handColor)
	red := lipgloss.NewStyle().Foreground(warnColor)
	cur := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(handColor)

	// Query row, with the caret parked at the end.
	q := m.query + " "
	rows := []string{
		hand.Render(" "+glyphSearch+" ") + txt.Render(m.query) +
			cur.Render(" ") + strings.Repeat(" ", max(0, innerW-4-dispW(q))),
		dim.Render(strings.Repeat("─", innerW)),
	}

	switch {
	case m.note != "" && len(m.matches) == 0:
		rows = append(rows, dim.Render(padRight("  "+m.note, innerW)))
	case len(m.matches) == 0:
		rows = append(rows, dim.Render(padRight("  no match", innerW)))
	}

	// mode(4) + two gaps + size(8), right-aligned; the name takes the rest.
	const metaW = 4 + 2 + 8
	nameW := innerW - metaW - 3
	end := min(len(m.matches), m.top+m.visible())
	for i := m.top; i < end; i++ {
		e := m.entries[m.matches[i]]
		name := padRight(e.label, nameW)
		// Both branches compose the meta columns the same way. Building them
		// differently is how the mode and size shift sideways as the cursor moves.
		modeCell := padLeft(fmt.Sprintf("%04o", e.mode.Perm()), 4)
		sizeCell := padLeft(humanSize(e.size), 8)

		if i == m.cursor {
			rows = append(rows, cur.Render(" "+name+" "+modeCell+"  "+sizeCell+" "))
			continue
		}
		// A key readable by group or other is one ssh will refuse — worth
		// flagging here, where the user is choosing it (§2.4 override colour).
		modeStyle := dim
		if e.mode.Perm()&0o077 != 0 {
			modeStyle = red
		}
		rows = append(rows, " "+txt.Render(name)+" "+
			modeStyle.Render(modeCell)+dim.Render("  "+sizeCell)+" ")
	}
	if m.note != "" && len(m.matches) > 0 {
		rows = append(rows, dim.Render(padRight("  "+m.note, innerW)))
	}

	title := " " + glyphKey + " Identity file  " + store.FoldHome(m.root) + " "
	hint := hintLegend([][2]string{{arrowUpDown, "select"}, {"Enter", "pick"}, {"Esc", "cancel"}})
	return drawPopupBoxPad(popupLayerColor(m.layer), title, hint,
		animRows(m.anim, capRows(rows, m.screenH)), innerW, false)
}

func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f kB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
	}
}
