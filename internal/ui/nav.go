package ui

// List navigation, in one place because it is one vocabulary: every list in the
// app answers to the same keys, and a letter hotkey may not quietly take one of
// them away.

// navKeys is that vocabulary. It exists as a set, and not just as the cases in
// moveCursor, because hotkeyIndex has to consult it:
//
// `u` and `d` move half a page, and `[U]nmark` / `[D]elete host` / `[D]uplicate`
// are declared in upper case. Without this set the case-insensitive fallback
// would let a press of `d` fold onto Delete and the list would stop scrolling —
// silently, and only on the panels that happen to carry such an action.
// Reserving the letters makes "lower case moves, upper case acts" true by
// construction rather than by remembering.
//
// Nothing may take one of these letters. tab [2]'s Delete was briefly on `d`,
// which cost that tab its half-page and needed a rule of its own to justify;
// moving it to `x`/`X` put this back to a sentence with no exceptions.
var navKeys = map[string]bool{
	"j": true, "k": true, "down": true, "up": true,
	"u": true, "d": true, "ctrl+u": true, "ctrl+d": true,
	"g": true, "gg": true, "G": true,
	// h/l only mean something in tab [2] (left half / right half), but they are
	// reserved everywhere: a letter that navigates on one surface must not be an
	// action's fold-target on another, or the same key would do two unrelated
	// things depending on where you stand.
	"h": true, "l": true, "left": true, "right": true,
}

// moveCursor resolves one navigation key against a list of n items.
//
// page is how many rows are on screen. `u`/`d` move by HALF of it — filu's and
// vim's half-page: far enough that a long list is walkable, short enough that
// the rows you land on overlap the ones you just left, so you keep your place.
// ctrl+u / ctrl+d are the same movement under the vim spelling, for hands that
// reach for it and for the surfaces where the bare letter is taken.
//
// **j and k WRAP.** Off the bottom is the top. A list is a ring, and the last
// item is one keystroke from the first — which matters most on the short lists,
// where the alternative is holding k and watching nothing happen.
//
// **u and d do NOT wrap.** A half-page is a movement you aim, and one that
// silently teleports to the other end of the list is worse than one that stops.
// The same goes for gg and G, which are absolute by definition.
func moveCursor(cur, n int, k string, page int) int {
	if n == 0 {
		return 0
	}
	half := max(1, page/2)
	switch k {
	case "j", "down":
		return (cur + 1) % n
	case "k", "up":
		return (cur - 1 + n) % n
	case "d", "ctrl+d":
		return min(n-1, cur+half)
	case "u", "ctrl+u":
		return max(0, cur-half)
	case "gg":
		return 0
	case "G":
		return n - 1
	}
	return cur
}

// moveScroll is the same vocabulary against a VIEWPORT — a thing with a top and
// no cursor, like [6] history or the help text. It never wraps: a scroll that
// jumps back to the top when it reaches the bottom reads as a glitch, because
// there is no cursor that could have gone round.
//
// last is the furthest the top may go, so the final line stays on screen and
// the view never scrolls past its own end.
func moveScroll(top, last int, k string, page int) int {
	if last <= 0 {
		return 0
	}
	half := max(1, page/2)
	switch k {
	case "j", "down":
		top++
	case "k", "up":
		top--
	case "d", "ctrl+d":
		top += half
	case "u", "ctrl+u":
		top -= half
	case "gg":
		return 0
	case "G":
		return last
	}
	return clamp(top, 0, last)
}
