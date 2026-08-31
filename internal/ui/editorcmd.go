package ui

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

// Which editor, and how it is started.
//
// $EDITOR is shell syntax, not a program name: `code -w`, `vim -u NONE` and a
// path with a space in it all have to work. So it goes through sh, which is how
// git runs an editor too.
//
// The FILE is a positional parameter and is never interpolated into that script.
// That is not tidiness — the name comes off somebody else's machine, and a file
// called `; rm -rf ~` would otherwise be a command instead of an argument.

// fallbackEditor is a last resort, not a dependency. POSIX guarantees it exists,
// which is the only reason it is the one named.
const fallbackEditor = "vi"

// errNoEditor names the variable to set, because that is the whole of the fix.
var errNoEditor = errors.New("no editor found — set $EDITOR")

// resolveEditor returns the editor command line: what the user asked for, or vi.
func resolveEditor() (string, error) {
	for _, k := range []string{"VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v, nil
		}
	}
	if _, err := exec.LookPath(fallbackEditor); err != nil {
		return "", errNoEditor
	}
	return fallbackEditor, nil
}

// editorCommand builds the command that opens p.
func editorCommand(p string) (*exec.Cmd, error) {
	ed, err := resolveEditor()
	if err != nil {
		return nil, err
	}
	c := exec.Command("sh", "-c", ed+` "$@"`, "sh", p)
	c.Env = editorEnv()
	return c, nil
}

// editorTermVars name the OUTER terminal. They are dropped, not passed on.
//
// The editor runs inside vt10x, a basic VT100/xterm emulator. nvim and its
// relatives read these to decide the terminal is a capable one, then send
// queries — device attributes, colour reports — that vt10x never answers. The
// editor waits for a reply that is not coming and hangs on the way out. sshu
// cannot make the emulator answer, so it stops claiming to be the terminal that
// would be asked. (kbu runs `kubectl edit` through the same strip list, for the
// same reason.)
var editorTermVars = []string{
	"TERM", "COLORTERM",
	"TERM_PROGRAM", "TERM_PROGRAM_VERSION", "TERM_SESSION_ID",
	"KITTY_WINDOW_ID", "KITTY_PUBLIC_KEY",
	"ITERM_SESSION_ID", "ITERM_PROFILE",
	"LC_TERMINAL", "LC_TERMINAL_VERSION",
	"WEZTERM_EXECUTABLE", "WEZTERM_PANE",
	"GHOSTTY_RESOURCES_DIR",
}

func editorEnv() []string {
	drop := make(map[string]bool, len(editorTermVars))
	for _, k := range editorTermVars {
		drop[k] = true
	}
	src := os.Environ()
	env := make([]string, 0, len(src)+1)
	for _, v := range src {
		if eq := strings.IndexByte(v, '='); eq >= 0 && drop[v[:eq]] {
			continue
		}
		env = append(env, v)
	}
	// What the emulator actually implements — the same claim an ssh session
	// gets, and for the same reason.
	return append(env, "TERM=xterm-256color")
}
