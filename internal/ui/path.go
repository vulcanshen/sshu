package ui

import "strings"

// fitPath shortens a path to w cells the way filu does it — in stages, so the
// structure survives as long as possible instead of the head being lopped off:
//
//  1. it fits            ~/Documents/sideproj/app
//  2. lead segments to    ~/D/sideproj/app  ->  ~/D/s/app
//     their first rune    (the last segment is never shortened)
//  3. middle elided       ~/.../app
//  4. last segment alone  app          (hard-truncated as a floor)
//
// A path is read from the tail — the last segment is what you are looking at —
// but the head is what tells you where that is. Cutting from the left destroys
// the second before it has to; shortening the leading segments keeps a legible
// skeleton of both.
func fitPath(p string, w int) string {
	if w <= 0 {
		return ""
	}
	if dispW(p) <= w {
		return p
	}

	root := ""
	rest := p
	if strings.HasPrefix(p, "/") {
		root, rest = "/", strings.TrimPrefix(p, "/")
	}
	segs := strings.Split(rest, "/")
	join := func(ss []string) string { return root + strings.Join(ss, "/") }

	// Stage 2: shorten leading segments, one at a time, nearest the root first.
	for i := 0; i < len(segs)-1; i++ {
		segs[i] = firstRune(segs[i])
		if dispW(join(segs)) <= w {
			return join(segs)
		}
	}

	// Stage 3: keep the first and last, elide the middle.
	last := segs[len(segs)-1]
	if len(segs) > 2 {
		if cand := join([]string{segs[0], "…", last}); dispW(cand) <= w {
			return cand
		}
	}
	if cand := root + "…/" + last; dispW(cand) <= w {
		return cand
	}

	// Stage 4: the last segment on its own, truncated if even that is too long.
	return truncate(last, w)
}

func firstRune(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}

// foldHomePath writes a path back in ~ form against the side's own home, so a
// remote path reads the same way a local one does.
func foldHomePath(p, home string) string {
	if home == "" || home == "/" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+"/") {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
