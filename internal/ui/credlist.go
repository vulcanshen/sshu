package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// credsModel is preference → credentials: a table over credentials.yaml.
// Name, User, Auth — no host column, because a credential deliberately does
// not know where it will be used.
type credsModel struct {
	creds  []store.Credential
	cursor int
	top    int
	w, h   int
}

func (m *credsModel) setSize(w, h int) {
	m.w, m.h = w, h
	m.ensureVisible()
}

func (m credsModel) visibleRows() int { return max(1, m.h-2-headerRows) }

func (m credsModel) rowAt(i int) (store.Credential, bool) {
	if i < 0 || i >= len(m.creds) {
		return store.Credential{}, false
	}
	return m.creds[i], true
}

func (m *credsModel) ensureVisible() {
	if len(m.creds) == 0 || m.w == 0 {
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

func (m *credsModel) handleKey(k string) {
	if len(m.creds) == 0 {
		return
	}
	m.cursor = moveCursor(m.cursor, len(m.creds), k, m.visibleRows())
	m.ensureVisible()
}

func (m credsModel) status() string {
	if len(m.creds) == 0 {
		return "no credentials"
	}
	return plural(len(m.creds), "credential")
}

// credCols is the split at this width. Auth is fixed like the hosts table's;
// user goes before auth does, and the name is the last thing standing.
func credCols(w int) (name, user int, auth bool) {
	avail := w - 2
	auth = true
	fixed := colAuthW + colGap
	if avail-fixed < minNameW+minUserW+colGap {
		auth, fixed = false, 0
	}
	free := avail - fixed
	if free < minNameW+minUserW+colGap {
		return max(1, avail), 0, auth
	}
	user = max(minUserW, (free-colGap)*2/5)
	name = free - colGap - user
	return name, user, auth
}

func (m credsModel) view(title string, focused bool) string {
	innerW, innerH := m.w-2, m.h-2
	var body []string
	if len(m.creds) == 0 {
		body = emptyBody(innerW, innerH, "No credentials yet",
			emptyHint("Press [A] to add one — hosts can then say auth: credential", "[A]"))
	} else {
		body = m.tableBody(innerW, innerH)
	}
	return panelChrome(innerW, fitLines(body, innerW, innerH), title, focused)
}

func (m credsModel) tableBody(innerW, innerH int) []string {
	name, user, auth := credCols(innerW)
	dim := lipgloss.NewStyle().Foreground(dimColor)

	head := " " + padRight("Name", name)
	if user > 0 {
		head += strings.Repeat(" ", colGap) + padRight("User", user)
	}
	if auth {
		head += strings.Repeat(" ", colGap) + padRight("Auth", colAuthW)
	}
	out := []string{dim.Render(padRight(head, innerW))}

	for i := m.top; i < len(m.creds) && len(out) < innerH; i++ {
		out = append(out, m.row(m.creds[i], i == m.cursor, name, user, auth, innerW))
	}
	return out
}

func (m credsModel) row(c store.Credential, selected bool, name, user int, auth bool, innerW int) string {
	body := lipgloss.NewStyle().Foreground(textColor)
	sub := lipgloss.NewStyle().Foreground(dimColor)
	if selected {
		bar := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(rowSelColor)
		body, sub = bar, bar
	}

	row := body.Render(" " + padRight(truncate(c.Name, name), name))
	if user > 0 {
		row += sub.Render(strings.Repeat(" ", colGap) + padRight(truncate(c.User, user), user))
	}
	if auth {
		glyph, text := glyphLock, string(store.AuthPassword)
		if c.Auth == store.AuthPrivateKey {
			glyph, text = glyphKey, string(store.AuthPrivateKey)
		}
		row += sub.Render(strings.Repeat(" ", colGap) + padRight(glyph+" "+text, colAuthW))
	}
	plain := 1 + name
	if user > 0 {
		plain += colGap + user
	}
	if auth {
		plain += colGap + colAuthW
	}
	filler := strings.Repeat(" ", max(0, innerW-plain))
	if selected {
		return row + body.Render(filler)
	}
	return row + filler
}
