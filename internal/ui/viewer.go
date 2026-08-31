package ui

import (
	"bytes"
	"fmt"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/remote"
)

// `[v]iew` is filu's preview, moved into a popup and cut down to what a REMOTE
// file can honestly offer.
//
// filu previews images, archives and PDFs too. None of those survive the trip:
// an image would have to be downloaded whole and drawn with a terminal protocol
// this app does not speak, an archive listing means fetching the archive, and a
// PDF means fetching it and linking a parser. What is left is what actually
// answers the question you ask before transferring something — text, and enough
// of a binary to recognise it:
//
//	directory  ->  one level of its listing
//	text       ->  line-numbered, syntax-highlighted
//	binary     ->  xxd-style hex with an ASCII gutter
//
// One level for a directory, not filu's three: over SFTP every level is another
// round trip, and "what is in here" is answered by the first one.
//
// It is a viewport, not a list: nothing in it can be selected, so nothing in it
// can be acted on. j/k/u/d scroll (§4.2), and it does not wrap.

type viewKind int

const (
	viewText viewKind = iota
	viewBinary
	viewDir
)

// viewLoadedMsg carries a finished preview. The read AND the rendering happen
// off the update loop — the read because it crosses the network, the
// highlighting because 64 KiB through a lexer is not free and a frame should
// never wait on either.
type viewLoadedMsg struct {
	gen   int
	title string
	kind  viewKind
	lines []string
	note  string // set instead of lines when there is nothing to show
}

type viewerPopup struct {
	anim  popupAnimator
	gen   int
	title string
	kind  viewKind
	lines []string
	note  string
	top   int

	layer   int
	screenW int
	screenH int
}

func newViewerPopup() viewerPopup {
	return viewerPopup{anim: newPopupAnimator("viewer")}
}

func (m viewerPopup) isActive() bool      { return m.anim.isActive() }
func (m viewerPopup) isInteractive() bool { return m.anim.isInteractive() }
func (m *viewerPopup) close() tea.Cmd     { return m.anim.close() }
func (m *viewerPopup) setSize(w, h int)   { m.screenW, m.screenH = w, h }

// open shows the box immediately, with a note, and loads behind it. A remote
// read can take as long as the link takes; opening only once the bytes arrive
// would look like the key did nothing (the same lesson as the dial spinner).
func (m *viewerPopup) open(layer int, title string) tea.Cmd {
	m.gen++
	m.layer, m.title, m.top = layer, title, 0
	m.lines, m.note, m.kind = nil, "reading…", viewText
	return m.anim.open()
}

// onLoaded installs a finished preview, unless the user has moved on.
func (m *viewerPopup) onLoaded(msg viewLoadedMsg) {
	if msg.gen != m.gen {
		return
	}
	m.title, m.kind, m.lines, m.note, m.top = msg.title, msg.kind, msg.lines, msg.note, 0
}

func (m viewerPopup) rows() int { return max(1, m.screenH-6) }

func (m *viewerPopup) update(msg tea.KeyMsg) {
	if !m.anim.isInteractive() {
		return
	}
	m.top = moveScroll(m.top, max(0, len(m.lines)-m.rows()), msg.String(), m.rows())
}

// viewerW is the popup's width. Wider than the others: this one is showing file
// content, and code wrapped at 56 columns is code you cannot read.
const viewerW = 96

func (m viewerPopup) view() string {
	innerW := popupInnerW(m.screenW, viewerW)

	var rows []string
	switch {
	case m.note != "":
		rows = emptyBody(innerW, min(m.rows(), 5), m.note, nil)
	default:
		// Rows go through as they are: drawPopupBox clips every one of them
		// ANSI-aware, which is what a line of highlighted source needs — and
		// doing it twice would just be a second place to get it wrong.
		end := min(len(m.lines), m.top+m.rows())
		rows = append(rows, m.lines[min(m.top, len(m.lines)):end]...)
	}

	hint := [][2]string{{"j/k", "scroll"}, {"Esc", "close"}}
	if n := len(m.lines); n > m.rows() {
		hint = append([][2]string{{itoa(m.top + 1), "of " + itoa(n)}}, hint...)
	}
	return drawPopupBox(popupLayerColor(m.layer), " "+glyphSearch+" "+m.title+" ",
		hintLegend(hint), animRows(m.anim, capRows(rows, m.screenH)), innerW)
}

// ------------------------------------------------------------------ loading

// loadView reads and renders off the update loop.
func loadView(gen int, fsys remote.FS, p string, isDir bool) tea.Cmd {
	return func() tea.Msg {
		name := path.Base(p)
		if isDir {
			return dirView(gen, name, fsys, p)
		}
		data, err := remote.Peek(fsys, p, remote.PeekCap)
		if err != nil {
			return viewLoadedMsg{gen: gen, title: name, note: "unreadable"}
		}
		if len(data) == 0 {
			return viewLoadedMsg{gen: gen, title: name, note: "empty file"}
		}
		if isText(data) {
			lines := sanitizeLines(strings.Split(string(data), "\n"))
			if hl, ok := highlight(name, strings.Join(lines, "\n")); ok {
				lines = hl
			}
			return viewLoadedMsg{gen: gen, title: name, kind: viewText,
				lines: withLineNumbers(lines)}
		}
		return viewLoadedMsg{gen: gen, title: name, kind: viewBinary,
			lines: withLineNumbers(hexDump(data))}
	}
}

func dirView(gen int, name string, fsys remote.FS, p string) viewLoadedMsg {
	entries, err := fsys.List(p)
	if err != nil {
		return viewLoadedMsg{gen: gen, title: name, note: "unreadable"}
	}
	if len(entries) == 0 {
		return viewLoadedMsg{gen: gen, title: name, note: "empty directory"}
	}
	dim := lipgloss.NewStyle().Foreground(dimColor)
	blue := lipgloss.NewStyle().Foreground(focusColor)

	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		size := humanSize(e.Size)
		if e.IsDir {
			lines = append(lines, blue.Render(e.Name+"/")+dim.Render("  "+size))
			continue
		}
		lines = append(lines, e.Name+dim.Render("  "+size))
	}
	return viewLoadedMsg{gen: gen, title: name, kind: viewDir, lines: lines}
}

// ------------------------------------------------------------- content prep

// isText treats a sample as text when it has no NUL byte and is valid UTF-8.
func isText(data []byte) bool {
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	return utf8.Valid(data)
}

func sanitizeLines(lines []string) []string {
	for i, l := range lines {
		lines[i] = sanitizeLine(l)
	}
	return lines
}

// sanitizeLine makes a line safe to draw: tabs become spaces, CR is dropped, and
// every other control byte becomes a space.
//
// ESC is the one that matters, and it matters MORE here than it did in filu: the
// bytes are coming off someone else's machine. A file containing escape
// sequences would otherwise repaint the popup, the panels around it, or the
// terminal itself. The length cap is the same idea — a single pathological line
// should not be allowed to make width arithmetic expensive.
func sanitizeLine(s string) string {
	const maxRunes = 2000
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n >= maxRunes {
			break
		}
		switch {
		case r == '\t':
			b.WriteString("    ")
		case r == '\r':
			continue
		case r < 0x20 || r == 0x7f:
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
		n++
	}
	return b.String()
}

// withLineNumbers prefixes each line with a dim, right-aligned gutter. The
// gutter carries its own reset, so highlighted content after it keeps its colour.
func withLineNumbers(lines []string) []string {
	width := max(len(strconv.Itoa(len(lines))), 2)
	gutter := lipgloss.NewStyle().Foreground(dimColor)
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = gutter.Render(fmt.Sprintf("%*d ", width, i+1)) + l
	}
	return out
}

// hexDump renders data xxd-style: offset, 16 hex bytes, ASCII gutter.
func hexDump(data []byte) []string {
	var out []string
	for off := 0; off < len(data); off += 16 {
		row := data[off:min(off+16, len(data))]
		var hex, asc strings.Builder
		for i := range 16 {
			if i < len(row) {
				fmt.Fprintf(&hex, "%02x ", row[i])
				if c := row[i]; c >= 0x20 && c < 0x7f {
					asc.WriteByte(c)
				} else {
					asc.WriteByte('.')
				}
			} else {
				hex.WriteString("   ")
			}
		}
		out = append(out, fmt.Sprintf("%08x  %s|%s|", off, hex.String(), asc.String()))
	}
	return out
}
