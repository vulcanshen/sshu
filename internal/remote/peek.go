package remote

import (
	"io"
)

// PeekCap is how much of a file a preview reads. Every byte crosses the network
// on a remote side, so this is a transfer budget rather than a memory one: 64 KiB
// is enough to answer "is this the file I think it is" and small enough that
// pressing the key on a multi-gigabyte log is not a mistake you have to wait out.
const PeekCap = 64 * 1024

// Peek reads at most n bytes from the start of p.
//
// It is deliberately not Plan/CopyItem's path: nothing is written anywhere, and
// the caller wants the bytes in hand rather than streamed.
func Peek(f FS, p string, n int) ([]byte, error) {
	r, err := f.Open(p)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, int64(n)))
}
