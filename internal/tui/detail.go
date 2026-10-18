package tui

import (
	"strconv"
	"strings"

	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/store"
)

// A host, a credential, a ~/.ssh/config block or a known_hosts key, written
// out for reading: the details above a confirm's question ("Connect to X?",
// components/dialog/confirm) or the lines of a viewer.

// detailSection is a titled group of label/value rows.
type detailSection struct {
	title string
	rows  []detailRow
}

type detailRow struct {
	label, value string
	depth        int  // indent under the title, in steps of two cells
	warn         bool // a warning: the line is drawn in Peach
}

// maskedSecret is a stored password: a fixed width, so a look at a saved
// password does not publish how long it is.
const maskedSecret = "••••••••"

// sshDecides is an sshconfig host's empty Port or User: where the answer
// comes from, not a blank that reads as forgotten.
const sshDecides = "— ssh decides"

func orSSHDecides(v string) string {
	if v == "" {
		return sshDecides
	}
	return v
}

// detailLines lays sections out as lines, the labels in one column (two
// cells between label and value, components "spacing"), and reports which
// lines are warnings.
func detailLines(secs []detailSection) (lines []string, warn []int) {
	w := 0
	for _, s := range secs {
		for _, r := range s.rows {
			w = max(w, terminu.Width(r.label)+2*r.depth)
		}
	}
	for i, s := range secs {
		if i > 0 {
			lines = append(lines, "")
		}
		if s.title != "" {
			lines = append(lines, terminu.Upper(s.title))
		}
		for _, r := range s.rows {
			if r.warn {
				warn = append(warn, len(lines))
			}
			indent := strings.Repeat("  ", r.depth+1)
			switch {
			case r.label == "" && r.value == "":
				lines = append(lines, "")
			case r.label == "":
				lines = append(lines, indent+r.value)
			default:
				lines = append(lines, indent+terminu.Pad(r.label, w-2*r.depth)+"  "+r.value)
			}
		}
	}
	return lines, warn
}

// hostDetail is one host: what it connects to, and how. A credential host
// shows what it resolves to; ~/.ssh/config's word on it follows.
func hostDetail(h store.Host, creds []store.Credential, cfg store.SSHConfigFile, timeoutSecs int) []detailSection {
	port := ""
	if h.Port > 0 {
		port = strconv.Itoa(h.Port)
	}
	conn := detailSection{title: "Connection", rows: []detailRow{
		{label: "Name", value: h.Name},
		{label: "Host", value: h.Host},
		{label: "Port", value: orSSHDecides(port)},
	}}
	if h.Auth != store.AuthCredential {
		conn.rows = append(conn.rows, detailRow{label: "User", value: orSSHDecides(h.User)})
	}
	if len(h.Tags) > 0 {
		conn.rows = append(conn.rows, detailRow{label: "Tags", value: strings.Join(h.Tags, " ")})
	}
	auth := detailSection{title: "Auth", rows: []detailRow{{label: "Type", value: string(h.Auth)}}}
	switch h.Auth {
	case store.AuthPassword:
		auth.rows = append(auth.rows, detailRow{label: "Password", value: maskedSecret})
	case store.AuthPrivateKey:
		auth.rows = append(auth.rows, detailRow{label: "Identity file", value: h.IdentityFile})
	case store.AuthCredential:
		rh, err := store.Resolve(h, creds)
		if err != nil {
			auth.rows = append(auth.rows, detailRow{label: "Credential",
				value: h.Credential + " — missing, cannot connect", warn: true})
			break
		}
		auth.rows = append(auth.rows,
			detailRow{label: "Credential", value: h.Credential},
			detailRow{label: "User", value: rh.User})
		auth.rows = append(auth.rows, credSecretRow(rh.Auth, rh.IdentityFile)...)
	case store.AuthSSHConfig:
		auth.rows = append(auth.rows, detailRow{label: "Secrets", value: "none stored — ssh asks at connect time"})
	}
	return append([]detailSection{conn, auth}, sshConfigSections(h, creds, cfg, timeoutSecs)...)
}

// sshConfigSections is what ~/.ssh/config contributes to the host: one
// section per run of blocks from one file, in the order ssh reads them, so
// the first value of each keyword is the one that wins. What sshu puts on the
// command line beats the file, and a row it beats with a different value is
// marked.
func sshConfigSections(h store.Host, creds []store.Credential, cfg store.SSHConfigFile, timeoutSecs int) []detailSection {
	opts := cfg.Effective(h.Host)
	if len(opts) == 0 {
		if h.Auth != store.AuthSSHConfig {
			return nil
		}
		return []detailSection{{title: "~/.ssh/config", rows: []detailRow{
			{label: "Host " + h.Host, value: "no block matches — ssh will use its defaults", warn: true},
		}}}
	}
	rh := h
	if r, err := store.Resolve(h, creds); err == nil {
		rh = r
	}
	type sent struct{ value, phrase string }
	secs := strconv.Itoa(timeoutSecs)
	sends := map[string]sent{"connecttimeout": {secs, "-o ConnectTimeout=" + secs}}
	if rh.Port > 0 {
		p := strconv.Itoa(rh.Port)
		sends["port"] = sent{p, "-p " + p}
	}
	if rh.User != "" {
		sends["user"] = sent{rh.User, rh.User + "@"}
	}
	if rh.Auth == store.AuthPrivateKey && rh.IdentityFile != "" {
		sends["identityfile"] = sent{rh.IdentityFile, "-i " + rh.IdentityFile}
		sends["identitiesonly"] = sent{"yes", "-o IdentitiesOnly=yes"}
	}
	var out []detailSection
	curBlock, curFile := -1, -1
	for _, o := range opts {
		blk := cfg.Blocks[o.Block]
		if blk.File() != curFile {
			curFile, curBlock = blk.File(), -1
			out = append(out, detailSection{title: store.FoldHome(cfg.Path(curFile))})
		}
		sec := &out[len(out)-1]
		if o.Block != curBlock {
			curBlock = o.Block
			if len(sec.rows) > 0 {
				sec.rows = append(sec.rows, detailRow{})
			}
			sec.rows = append(sec.rows, detailRow{value: "Host " + o.From})
		}
		row := detailRow{label: o.Key, value: o.Value, depth: 1}
		if mine, ok := sends[strings.ToLower(o.Key)]; ok && !strings.EqualFold(mine.value, o.Value) {
			row.value, row.warn = o.Value+" — sshu sends "+mine.phrase, true
		}
		sec.rows = append(sec.rows, row)
	}
	var maybe []detailRow
	for _, inc := range cfg.Unread {
		maybe = append(maybe, detailRow{label: "Include", value: inc})
	}
	for _, cond := range cfg.MatchConditions {
		if cond == "" {
			cond = "(no condition)"
		}
		maybe = append(maybe, detailRow{label: "Match", value: cond + " — not evaluated"})
	}
	if len(maybe) > 0 {
		out = append(out, detailSection{title: "~/.ssh/config · may also apply", rows: maybe})
	}
	return out
}

// credDetail is one credential: the auth half, the whole of what one is.
func credDetail(c store.Credential) []detailSection {
	rows := []detailRow{{label: "Name", value: c.Name}, {label: "User", value: c.User}, {label: "Type", value: string(c.Auth)}}
	rows = append(rows, credSecretRow(c.Auth, c.IdentityFile)...)
	return []detailSection{{title: "Auth", rows: rows}}
}

func credSecretRow(auth store.AuthMethod, identity string) []detailRow {
	if auth == store.AuthPrivateKey {
		return []detailRow{{label: "Identity file", value: identity}}
	}
	return []detailRow{{label: "Password", value: maskedSecret}}
}
