package ui

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/store"
)

// The Operation pages. Export and Import are CONTENT PANELS that take text —
// pages, not popups: they are destinations the nav cursor lands on, exactly
// like hosts or the log, so hiding their one form behind another keypress
// would add a step with nothing standing on it. A page that types follows the
// same §4.5 split every text surface makes — letters (and digits) type, Tab
// moves fields, Enter submits — which costs the global vocabulary while one
// is focused: the digits cannot address panels and q cannot quit. Esc hands
// the keyboard back to the nav, and the footer switches to the keys that are
// still true.

const (
	exportIntro = "Bundle hosts.yaml + credentials.yaml into one " + store.BundleExt + " zip."
	exportWarn  = "The bundle carries the same plaintext passwords the YAML files do."
	importIntro = "Append a bundle's hosts and credentials to yours — existing names win, duplicates are skipped."
)

type bundlePage struct {
	fields []formField
	focus  int
	err    string
	errIdx int
	done   string // the last success, standing where the error would
}

// newExportPage: the output directory defaults to where sshu was launched —
// the same "local opens here" convention tab [2] uses — and the filename is
// the default's full spelling, extension included, so what will be written is
// exactly what the field shows.
func newExportPage() bundlePage {
	dir, _ := os.Getwd()
	f := []formField{
		{label: "Directory", value: store.FoldHome(dir)},
		{label: "Filename", value: "sshu-export" + store.BundleExt},
	}
	for i := range f {
		f[i].caret = len([]rune(f[i].value))
	}
	return bundlePage{fields: f, errIdx: -1}
}

// newImportPage: the path is typed, not picked. The file picker deliberately
// never walks $HOME (filepicker.go), and a .sshu that just crossed machines
// has a path the user is already holding.
func newImportPage() bundlePage {
	return bundlePage{
		fields: []formField{{label: "Bundle", placeholder: "path to a " + store.BundleExt + " bundle"}},
		errIdx: -1,
	}
}

func (p *bundlePage) moveFocus(d int) {
	p.focus = (p.focus + d + len(p.fields)) % len(p.fields)
}

// fail parks the error on the page and moves the cursor to the offending
// field — hostForm.fail's contract (§6.7): visible without blocking the fix.
func (p *bundlePage) fail(msg string, field int) {
	p.err, p.errIdx, p.done = msg, field, ""
	if field >= 0 && field < len(p.fields) {
		p.focus = field
	}
}

// body renders the page: an intro naming what Enter will do, the fields, one
// standing status row (the error OR the last success — reserved even when
// blank, so the page never changes shape), the passwords warning where it
// applies, and the standing key disclosure a text surface owes (§4.5).
func (p bundlePage) body(intro, warn, submitLabel string, focused bool, innerW int) []string {
	dim := lipgloss.NewStyle().Foreground(dimColor)
	red := lipgloss.NewStyle().Foreground(warnColor)
	live := lipgloss.NewStyle().Foreground(liveColor)
	txt := lipgloss.NewStyle().Foreground(textColor)
	edit := lipgloss.NewStyle().Foreground(editColor)

	labelW := 0
	for _, f := range p.fields {
		labelW = max(labelW, dispW(f.label))
	}
	labelCol := min(labelW+4, max(0, innerW-8))
	valueW := max(0, innerW-labelCol-1)

	rows := []string{"", dim.Render(truncate("  "+intro, innerW)), ""}
	for i, f := range p.fields {
		lStyle := txt
		switch {
		case i == p.errIdx:
			lStyle = red
		case focused && i == p.focus:
			lStyle = edit
		}
		rows = append(rows, lStyle.Render(padRight("  "+f.label, labelCol))+
			renderTextValue(f, focused && i == p.focus, valueW))
	}
	status := ""
	switch {
	case p.err != "":
		status = red.Render(truncate("  "+p.err, innerW))
	case p.done != "":
		status = live.Render(truncate("  "+p.done, innerW))
	}
	rows = append(rows, "", status, "")
	if warn != "" {
		rows = append(rows, dim.Render(truncate("  "+warn, innerW)), "")
	}
	pairs := [][2]string{{"Enter", submitLabel}, {"Esc", "back to the nav"}}
	if len(p.fields) > 1 {
		pairs = append([][2]string{{"Tab", "next"}}, pairs...)
	}
	return append(rows, clipANSI("  "+hintLegend(pairs), innerW))
}

// textPage reports whether the keyboard is standing in an Operation page: a
// text-entry surface that is a panel rather than a popup, claiming printable
// keys the way a text float does (§4.5).
func (m AppModel) textPage() bool {
	return m.tab == tabPref && m.pref.focus == panelPrefContent &&
		(m.pref.item == prefExport || m.pref.item == prefImport) && !m.popupOpen()
}

// bundlePageKey routes one keystroke into the focused Operation page.
func (m AppModel) bundlePageKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &m.exportPage
	if m.pref.item == prefImport {
		p = &m.importPage
	}
	switch msg.Type {
	case tea.KeyTab, tea.KeyDown:
		p.moveFocus(1)
		return m, nil
	case tea.KeyShiftTab, tea.KeyUp:
		p.moveFocus(-1)
		return m, nil
	case tea.KeyEnter:
		if m.pref.item == prefExport {
			return m.doExport()
		}
		return m.doImport()
	}
	if msg.Alt {
		return m, nil // Alt keys are commands, not characters — same as the form
	}
	if editField(&p.fields[p.focus], msg) {
		// The row changed; the old verdict no longer describes it.
		p.err, p.errIdx, p.done = "", -1, ""
	}
	return m, nil
}

// doExport zips the CURRENT lists — what the tables show, not a re-read of
// disk — into dir/filename. The extension is appended when missing rather
// than rejected: .sshu is the only thing this page writes.
func (m AppModel) doExport() (tea.Model, tea.Cmd) {
	p := &m.exportPage
	dir := strings.TrimSpace(p.fields[0].value)
	name := strings.TrimSpace(p.fields[1].value)
	switch {
	case dir == "":
		p.fail("directory is required", 0)
		return m, nil
	case name == "":
		p.fail("filename is required", 1)
		return m, nil
	case len(m.hosts.hosts) == 0 && len(m.creds.creds) == 0:
		p.fail("nothing to export yet — add a host first", -1)
		return m, nil
	}
	if !strings.HasSuffix(name, store.BundleExt) {
		name += store.BundleExt
	}
	path := filepath.Join(store.ExpandTilde(dir), name)
	if err := store.ExportBundle(path, store.File{Hosts: m.hosts.hosts},
		store.CredsFile{Credentials: m.creds.creds}); err != nil {
		p.fail(err.Error(), -1)
		return m, nil
	}
	sum := plural(len(m.hosts.hosts), "host") + " · " + plural(len(m.creds.creds), "credential")
	p.err, p.errIdx = "", -1
	p.done = "Exported " + sum + " → " + store.FoldHome(path)
	m.log.info("exported " + sum + " to " + store.FoldHome(path))
	return m, m.toast.show("Exported → "+store.FoldHome(path), toastInfo)
}

// doImport reads the bundle, merges under the Name-is-key rule (store.Merge*)
// and persists both files. Credentials land first: an imported host may name
// an imported credential, and the other order could leave a dangling
// reference if the second save failed.
func (m AppModel) doImport() (tea.Model, tea.Cmd) {
	p := &m.importPage
	path := strings.TrimSpace(p.fields[0].value)
	if path == "" {
		p.fail("the bundle path is required", 0)
		return m, nil
	}
	in, inCreds, err := store.ImportBundle(store.ExpandTilde(path))
	if err != nil {
		p.fail(err.Error(), 0)
		return m, nil
	}
	creds, addedC, skippedC := store.MergeCreds(m.creds.creds, inCreds.Credentials)
	hosts, addedH, skippedH := store.MergeHosts(m.hosts.hosts, in.Hosts)
	if addedC > 0 && m.saveCreds != nil {
		if err := m.saveCreds(creds); err != nil {
			p.fail(err.Error(), -1)
			return m, nil
		}
	}
	if addedH > 0 {
		if err := m.persist(hosts); err != nil {
			p.fail(err.Error(), -1)
			return m, nil
		}
	}
	m.creds.creds, m.hosts.creds = creds, creds
	m.hosts.hosts = hosts

	var sum string
	switch {
	case addedH+addedC > 0:
		sum = "Imported " + plural(addedH, "host") + " · " + plural(addedC, "credential")
		if sk := skippedH + skippedC; sk > 0 {
			sum += " (" + itoa(sk) + " skipped)"
		}
	case skippedH+skippedC > 0:
		sum = "Nothing new — " + itoa(skippedH+skippedC) + " skipped (already here or invalid)"
	default:
		sum = "The bundle was empty"
	}
	p.err, p.errIdx = "", -1
	p.done = sum
	m.log.info(strings.ToLower(sum[:1]) + sum[1:] + " from " + store.FoldHome(store.ExpandTilde(path)))
	return m, m.toast.show(sum, toastInfo)
}
