package remote

import (
	"context"
	"strings"
)

// SearchCap bounds a scan, in the same spirit as planCap: past this many entries
// a search is not the tool you want, and the walk stops rather than holding the
// connection open indefinitely. The UI says when it stopped early, because a
// truncated result set that claims to be complete is worse than a slow one.
const SearchCap = 20000

// Scan walks dirs breadth-first and hands each directory's listing to emit, with
// every entry's Name rewritten as its path relative to root — so the caller can
// still build an absolute path with Join(root, Name) and does not have to know
// how deep a result came from.
//
// Breadth-first is the whole point. Over SFTP each directory is a round trip, so
// the order results arrive in is the order the user waits in, and what someone
// is looking for is far more often two levels down than twenty. A depth-first
// walk would spend its first seconds inside whichever subtree happens to sort
// first, which from the outside is indistinguishable from being stuck.
//
// A directory that will not list is skipped, not fatal. One unreadable directory
// is ordinary on someone else's machine, and dropping every result below it is
// not a reasonable answer to it.
//
// It returns when the queue empties, when ctx is cancelled, or at limit; capped
// reports which of the last two ended it.
func Scan(ctx context.Context, f FS, root string, dirs []string, limit int, emit func([]Entry)) (capped bool) {
	queue := append([]string(nil), dirs...)
	n := 0

	for len(queue) > 0 {
		if ctx.Err() != nil {
			return false
		}
		dir := queue[0]
		queue = queue[1:]

		entries, err := f.List(dir)
		if err != nil {
			continue
		}

		batch := make([]Entry, 0, len(entries))
		for _, e := range entries {
			p := Join(dir, e.Name)
			if e.IsDir {
				queue = append(queue, p)
			}
			e.Name = relTo(root, p)
			batch = append(batch, e)

			n++
			if n >= limit {
				emit(batch)
				return true
			}
		}
		if len(batch) > 0 {
			emit(batch)
		}
	}
	return false
}

// relTo writes p against root. Both are absolute slash paths and p is known to
// be below root, so this is a prefix strip rather than a general relativiser.
func relTo(root, p string) string {
	if root == "/" {
		return strings.TrimPrefix(p, "/")
	}
	return strings.TrimPrefix(p, root+"/")
}
