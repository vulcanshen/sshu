package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// knownModel is manage → SSH → KnownHosts: a table over ~/.ssh/known_hosts.
//
// sshu already reads this file — remote/sftp.go refuses a connection outright
// when a known host's key has changed, deliberately without offering a yes/no.
// This panel is the way out of that refusal, and until now the only way out was
// `ssh-keygen -R` in another terminal.
type knownModel struct {
	file   store.KnownHostsFile
	cursor int
	top    int
	w, h   int
}

const (
	// knownTypeW holds the short form of every key type in circulation:
	// "ecdsa-256" is nine, "sk-ed25519" ten.
	knownTypeW = 10
	// minFpW is enough of a fingerprint to tell two rows apart. The whole of it
	// is 43 characters and belongs in the float, not in a column.
	minFpW = 12
)

func (m *knownModel) setSize(w, h int) {
	m.w, m.h = w, h
	m.ensureVisible()
}

func (m knownModel) visibleRows() int { return max(1, m.h-2-headerRows) }

func (m knownModel) rowAt(i int) (store.KnownHostEntry, bool) {
	if i < 0 || i >= len(m.file.Entries) {
		return store.KnownHostEntry{}, false
	}
	return m.file.Entries[i], true
}

func (m *knownModel) ensureVisible() {
	if len(m.file.Entries) == 0 || m.w == 0 {
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

func (m *knownModel) handleKey(k string) {
	if len(m.file.Entries) == 0 {
		return
	}
	m.cursor = moveCursor(m.cursor, len(m.file.Entries), k, m.visibleRows())
	m.ensureVisible()
}

func (m knownModel) status() string {
	if len(m.file.Entries) == 0 {
		return "no host keys"
	}
	out := itoa(m.cursor+1) + "/" + itoa(len(m.file.Entries)) + " keys"
	// Hashed names are a fact about what this panel can and cannot show, so it
	// says how many rows are in that state rather than letting the "(hashed)"
	// cells look like a rendering fault.
	n := 0
	for _, e := range m.file.Entries {
		if e.Hashed() {
			n++
		}
	}
	if n > 0 {
		out += " · " + itoa(n) + " hashed"
	}
	return out
}

// knownHostLabel is what the host column shows.
//
// A hashed name CANNOT be shown: it is an HMAC, and the whole point of hashing
// it is that the file no longer says where you have been. "(hashed)" is the
// honest answer; the fingerprint column is what tells those rows apart.
//
// A marker rides in front of the name because `@revoked` inverts what the row
// means, and a row that reads as trusted when it is the opposite is the one
// mistake this panel must not make.
func knownHostLabel(e store.KnownHostEntry) string {
	name := e.Hosts
	if e.Hashed() {
		name = "(hashed)"
	}
	if e.Marker != "" {
		return e.Marker + " " + name
	}
	return name
}

// shortKeyType trims the noise every key type carries. The table is comparing
// them to each other, and "ssh-" is on all of them.
func shortKeyType(t string) string {
	s := strings.TrimSuffix(t, "@openssh.com")
	s = strings.TrimPrefix(s, "sk-")
	switch {
	case strings.HasPrefix(s, "ecdsa-sha2-nistp"):
		s = "ecdsa-" + strings.TrimPrefix(s, "ecdsa-sha2-nistp")
	case s == "ssh-dss":
		s = "dsa"
	default:
		s = strings.TrimPrefix(s, "ssh-")
	}
	if strings.HasPrefix(t, "sk-") {
		return "sk-" + s
	}
	return s
}

// shortFingerprint drops the "SHA256:" everything here shares. What is left is
// the part that differs, which is the only part a column can be used for.
func shortFingerprint(fp string) string { return strings.TrimPrefix(fp, "SHA256:") }

// knownCols is the split at this width. Type goes first when space runs out —
// it is the least useful thing to scan — then the fingerprint, and the host is
// last standing because a key with no name on screen is not a row.
func knownCols(w int) (host, typ, fp int) {
	avail := max(0, w-2)
	if body := avail - 2*colGap - knownTypeW; body >= minNameW+minFpW {
		fp = max(minFpW, body*45/100)
		return body - fp, knownTypeW, fp
	}
	if body := avail - colGap; body >= minNameW+minFpW {
		fp = max(minFpW, body*45/100)
		return body - fp, 0, fp
	}
	return max(1, avail), 0, 0
}

func (m knownModel) view(title string, focused bool) string {
	innerW, innerH := m.w-2, m.h-2
	var body []string
	if len(m.file.Entries) == 0 {
		body = emptyBody(innerW, innerH, "Nothing in ~/.ssh/known_hosts",
			emptyHint("Press [A] to fetch a host's key, or Space to see what you can do here",
				"[A]", "Space"))
	} else {
		body = m.tableBody(innerW, innerH)
	}
	return panelChrome(innerW, fitLines(body, innerW, innerH), title, focused)
}

func (m knownModel) tableBody(innerW, innerH int) []string {
	host, typ, fp := knownCols(innerW)
	dim := lipgloss.NewStyle().Foreground(dimColor)

	head := " " + padRight("Host", host)
	if typ > 0 {
		head += strings.Repeat(" ", colGap) + padRight("Type", typ)
	}
	if fp > 0 {
		head += strings.Repeat(" ", colGap) + padRight("Fingerprint", fp)
	}
	out := []string{dim.Render(padRight(head, innerW))}

	for i := m.top; i < len(m.file.Entries) && len(out) < innerH; i++ {
		out = append(out, m.row(m.file.Entries[i], i == m.cursor, host, typ, fp, innerW))
	}
	return out
}

func (m knownModel) row(e store.KnownHostEntry, selected bool, host, typ, fp, innerW int) string {
	body := lipgloss.NewStyle().Foreground(textColor)
	sub := lipgloss.NewStyle().Foreground(dimColor)
	name := body
	// A revoked key is the opposite of a trusted one. It wears the warning
	// colour for the same reason a missing credential does: the row has to say
	// what it is before anybody acts on it.
	if e.Marker == "@revoked" && !selected {
		name = lipgloss.NewStyle().Foreground(warnColor)
	}
	if selected {
		bar := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(rowSelColor)
		body, sub, name = bar, bar, bar
	}

	row := name.Render(" " + padRight(truncate(knownHostLabel(e), host), host))
	plain := 1 + host
	if typ > 0 {
		row += sub.Render(strings.Repeat(" ", colGap) +
			padRight(truncate(shortKeyType(e.Type), typ), typ))
		plain += colGap + typ
	}
	if fp > 0 {
		row += sub.Render(strings.Repeat(" ", colGap) +
			padRight(truncate(shortFingerprint(e.Fingerprint()), fp), fp))
		plain += colGap + fp
	}

	filler := strings.Repeat(" ", max(0, innerW-plain))
	if selected {
		return row + body.Render(filler)
	}
	return row + filler
}
