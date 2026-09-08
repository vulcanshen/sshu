package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// sshcfgModel is manage → SSH → Config: a table over the Host blocks in
// ~/.ssh/config.
//
// It is the one panel in sshu whose file belongs to somebody else. Tab [3]
// shells out to the real ssh binary, so these options already decide what a
// sshu session does — HostName, ProxyJump and the rest arrive without sshu
// passing them. Tab [2]/[4] speak SFTP themselves and read none of it. This
// panel is where that file stops being invisible.
type sshcfgModel struct {
	file   store.SSHConfigFile
	cursor int
	top    int
	w, h   int
}

// minFileW is the floor for the column that says which file a block lives in.
// It shows the WHOLE path, not the base name: two included files called `work`
// under different directories would otherwise render as the same row, which is
// the one thing this column exists to prevent. It appears only when Include
// actually brought in a second file — a column reading `~/.ssh/config` on every
// row of a one-file config is a column that says nothing.
//
// There is no Opts column any more. It counted a block's option lines, which is
// a fact the detail float states properly and a table can only hint at; the
// width it took is better spent on the path.
const minFileW = 10

// minTargetW is the HostName column's floor — enough for an octet and a dot.
const minTargetW = 8

func (m *sshcfgModel) setSize(w, h int) {
	m.w, m.h = w, h
	m.ensureVisible()
}

func (m sshcfgModel) visibleRows() int { return max(1, m.h-2-headerRows) }

func (m sshcfgModel) rowAt(i int) (store.SSHBlock, bool) {
	if i < 0 || i >= len(m.file.Blocks) {
		return store.SSHBlock{}, false
	}
	return m.file.Blocks[i], true
}

func (m *sshcfgModel) ensureVisible() {
	if len(m.file.Blocks) == 0 || m.w == 0 {
		m.top = 0
		return
	}
	vis := m.visibleRows()
	if m.cursor < m.top {
		m.top = m.cursor
	}
	if m.cursor >= m.top+vis {
		m.top = m.cursor - vis + 1
	}
	m.top = max(0, m.top)
}

func (m *sshcfgModel) handleKey(k string) {
	if len(m.file.Blocks) == 0 {
		return
	}
	m.cursor = moveCursor(m.cursor, len(m.file.Blocks), k, m.visibleRows())
	m.ensureVisible()
}

// status names what is on screen AND what is not. Include is followed now, so
// the count spans files and says how many — and an Include that resolved to
// nothing is still disclosed, because a list that quietly drops a source reads
// as the whole picture (§11.39).
func (m sshcfgModel) status() string {
	var parts []string
	if len(m.file.Blocks) == 0 {
		parts = append(parts, "no Host blocks")
	} else {
		out := itoa(m.cursor+1) + "/" + itoa(len(m.file.Blocks)) + " blocks"
		if n := m.file.Files(); n > 1 {
			out += " in " + plural(n, "file")
		}
		parts = append(parts, out)
	}
	if n := len(m.file.Unread); n > 0 {
		parts = append(parts, plural(n, "include")+" unread")
	}
	return strings.Join(parts, " · ")
}

// sshcfgCols is the split at this width. Columns are dropped nicety-first: Opts
// goes before User, and Host is last standing because a row with no pattern is
// not a row.
func sshcfgCols(w, files int) (host, target, user, file int) {
	avail := max(0, w-2)
	const three = minNameW + minTargetW + minUserW + 2*colGap
	const four = minNameW + minTargetW + minUserW + minFileW + 3*colGap

	// File is a flex column, not a fixed one: a path is as long as it is, and a
	// fixed narrow slot would cut off exactly the tail that tells two of them
	// apart. It is also the first to go when the terminal cannot hold four —
	// the detail float still says which file, and a row with no pattern is not
	// a row at all.
	if files > 1 && avail >= four {
		body := avail - 3*colGap
		user = max(minUserW, body*15/100)
		file = max(minFileW, body*25/100)
		target = max(minTargetW, (body-user-file)*50/100)
		return body - user - file - target, target, user, file
	}
	if avail >= three {
		host, target, user = splitThree(avail - 2*colGap)
		return host, target, user, 0
	}
	if avail >= minNameW+colGap+minTargetW {
		target = max(minTargetW, (avail-colGap)*45/100)
		return avail - colGap - target, target, 0, 0
	}
	return max(1, avail), 0, 0, 0
}

// splitThree divides the text budget between Host, HostName and User. User is
// the narrowest because a login name is short and a pattern is not; HostName
// takes half of what is left because it is the answer to the question the row
// is being asked — where does this actually go.
func splitThree(body int) (host, target, user int) {
	user = max(minUserW, body*20/100)
	target = max(minTargetW, (body-user)*50/100)
	return body - user - target, target, user
}

func (m sshcfgModel) view(title string, focused bool) string {
	innerW, innerH := m.w-2, m.h-2
	var body []string
	if len(m.file.Blocks) == 0 {
		body = m.emptyState(innerW, innerH)
	} else {
		body = m.tableBody(innerW, innerH)
	}
	return panelChrome(innerW, fitLines(body, innerW, innerH), title, focused)
}

// emptyState names the FILE, because "nothing here" on a panel called Config
// would be ambiguous with sshu's own config.yaml — and because an Include that
// resolved to nothing means the file was TRYING to do something, which is a
// different kind of empty from never having been written.
func (m sshcfgModel) emptyState(innerW, innerH int) []string {
	fact := "Nothing in ~/.ssh/config"
	if n := len(m.file.Unread); n > 0 {
		fact = "No Host blocks — " + plural(n, "include") + " resolved to nothing"
	}
	return emptyBody(innerW, innerH, fact,
		emptyHint("Press [A] to add one, or Space to see what you can do here",
			"[A]", "Space"))
}

func (m sshcfgModel) tableBody(innerW, innerH int) []string {
	host, target, user, file := sshcfgCols(innerW, m.file.Files())
	dim := lipgloss.NewStyle().Foreground(dimColor)

	head := " " + padRight("Host", host)
	if target > 0 {
		head += strings.Repeat(" ", colGap) + padRight("HostName", target)
	}
	if user > 0 {
		head += strings.Repeat(" ", colGap) + padRight("User", user)
	}
	if file > 0 {
		head += strings.Repeat(" ", colGap) + padRight("File", file)
	}
	out := []string{dim.Render(padRight(head, innerW))}

	for i := m.top; i < len(m.file.Blocks) && len(out) < innerH; i++ {
		out = append(out, m.row(m.file.Blocks[i], i == m.cursor,
			host, target, user, file, innerW))
	}
	return out
}

// row is one Host block. A `Host *` row is mostly blank — it sets no HostName
// and no User — and that is honest: what it DOES set has no column, and the
// detail float is where a block's contents belong.
func (m sshcfgModel) row(b store.SSHBlock, selected bool, host, target, user, file, innerW int) string {
	body := lipgloss.NewStyle().Foreground(textColor)
	sub := lipgloss.NewStyle().Foreground(dimColor)
	if selected {
		bar := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(rowSelColor)
		body, sub = bar, bar
	}

	row := body.Render(" " + padRight(truncate(b.Patterns, host), host))
	plain := 1 + host
	if target > 0 {
		row += sub.Render(strings.Repeat(" ", colGap) +
			padRight(truncate(b.Option("HostName"), target), target))
		plain += colGap + target
	}
	if user > 0 {
		row += sub.Render(strings.Repeat(" ", colGap) +
			padRight(truncate(b.Option("User"), user), user))
		plain += colGap + user
	}
	if file > 0 {
		row += sub.Render(strings.Repeat(" ", colGap) +
			padRight(truncateHead(m.file.Path(b.File()), file), file))
		plain += colGap + file
	}

	filler := strings.Repeat(" ", max(0, innerW-plain))
	if selected {
		return row + body.Render(filler)
	}
	return row + filler
}
