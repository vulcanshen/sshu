package tui

// Nerd Font glyphs sshu picks itself: the domain's (an auth method, a file
// type) and the titles of its inputs, forms and notes (components/layout/popup:
// the app's choice). Written as code points (tdp E4).
const (
	glyphKey  = "\U0000F084" // nf-fa-key: privatekey
	glyphLock = "\U0000F023" // nf-fa-lock: password
	glyphCred = "\U000F05D2" // nf-md-card_account_details: credential
	glyphCog  = "\U000F107C" // nf-md-file_cog_outline: sshconfig, ~/.ssh/config

	glyphPlus    = "\U0000F067" // nf-fa-plus: a new host, credential, block
	glyphPencil  = "\U0000F040" // nf-fa-pencil: an edit, a value
	glyphConnect = "\U0000F120" // nf-fa-terminal: a host's details
	glyphShield  = "\U000F0483" // nf-md-shield_key: a known_hosts key
	glyphLog     = "\U000F02DA" // nf-md-history: a journal
	glyphDir     = "\U0000E5FF" // nf-custom-folder
	glyphFile    = "\U0000F15B" // nf-fa-file
	glyphEye     = "\U000F06D0" // nf-md-eye_outline: a file viewed
	glyphJobs    = "\U000F1065" // nf-md-transfer: transfers
	glyphGrid    = "\U000F0570" // nf-md-view_grid: the grid's columns
	glyphHost    = "\U000F048B" // nf-md-server: a host to pick
)

// authGlyph is the glyph of an auth method in a list.
func authGlyph(method string) string {
	switch method {
	case "password":
		return glyphLock
	case "privatekey":
		return glyphKey
	case "credential":
		return glyphCred
	case "sshconfig":
		return glyphCog
	}
	return " "
}
