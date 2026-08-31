package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
)

// Item is one thing to create at the far end. Directories are items too: an
// empty directory has to arrive, and a file cannot be written before its parent
// exists.
type Item struct {
	Src   string
	Dst   string
	Size  int64
	Mode  fs.FileMode
	IsDir bool
}

// planCap bounds the walk. A transfer of a hundred thousand files is a mistake
// somebody is about to make, and finding out after twenty minutes of walking is
// worse than being told now.
const planCap = 20000

// ErrPlanTooLarge says the walk hit the cap. The count is included so the
// message can be specific rather than "too many".
var ErrPlanTooLarge = errors.New("too many files")

// Plan expands paths into the items a transfer will create under dstDir,
// recursing into directories, and returns the total bytes.
//
// It runs before anything is written so the whole shape of the job is known up
// front: how many files, how many bytes, and which of them already exist at the
// far end. Discovering an overwrite mid-copy would mean asking a question with
// half the batch already committed.
func Plan(src FS, paths []string, dstDir string) ([]Item, int64, error) {
	var items []Item
	var total int64

	for _, p := range paths {
		e, err := src.Stat(p)
		if err != nil {
			return nil, 0, fmt.Errorf("%s: %w", path.Base(p), err)
		}
		dst := Join(dstDir, path.Base(p))
		if !e.IsDir {
			items = append(items, Item{Src: p, Dst: dst, Size: e.Size, Mode: e.Mode})
			total += e.Size
			continue
		}
		items = append(items, Item{Src: p, Dst: dst, IsDir: true})
		sub, n, err := walk(src, p, dst, len(items))
		if err != nil {
			return nil, 0, err
		}
		items = append(items, sub...)
		total += n
	}
	if len(items) > planCap {
		return nil, 0, fmt.Errorf("%w: %d, refusing to start", ErrPlanTooLarge, len(items))
	}
	return items, total, nil
}

func walk(src FS, dir, dstDir string, seen int) ([]Item, int64, error) {
	entries, err := src.List(dir)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", dir, err)
	}
	var items []Item
	var total int64
	for _, e := range entries {
		if seen+len(items) > planCap {
			return nil, 0, fmt.Errorf("%w: over %d", ErrPlanTooLarge, planCap)
		}
		s, d := Join(dir, e.Name), Join(dstDir, e.Name)
		if e.IsDir {
			items = append(items, Item{Src: s, Dst: d, IsDir: true})
			sub, n, err := walk(src, s, d, seen+len(items))
			if err != nil {
				return nil, 0, err
			}
			items = append(items, sub...)
			total += n
			continue
		}
		items = append(items, Item{Src: s, Dst: d, Size: e.Size, Mode: e.Mode})
		total += e.Size
	}
	return items, total, nil
}

// Conflicts are the items whose destination already exists.
func Conflicts(dst FS, items []Item) []Item {
	var out []Item
	for _, it := range items {
		if !it.IsDir && Exists(dst, it.Dst) {
			out = append(out, it)
		}
	}
	return out
}

// copyBuf is the transfer block size. 256 KiB keeps the SFTP pipeline fed
// without making the progress counter jump in visible steps.
const copyBuf = 256 * 1024

// CopyItem transfers one item, reporting bytes as they land.
//
// A cancelled or failed file is REMOVED. A partial file that looks like the real
// one is the worst outcome here: the next thing that reads it gets truncated
// data with nothing to say it was truncated.
func CopyItem(ctx context.Context, src, dst FS, it Item, onBytes func(int64)) error {
	if it.IsDir {
		return dst.MkdirAll(it.Dst)
	}
	if err := dst.MkdirAll(path.Dir(it.Dst)); err != nil {
		return err
	}

	r, err := src.Open(it.Src)
	if err != nil {
		return fmt.Errorf("%s: %w", path.Base(it.Src), err)
	}
	defer r.Close()

	w, err := dst.Create(it.Dst, it.Mode)
	if err != nil {
		return fmt.Errorf("%s: %w", path.Base(it.Dst), err)
	}

	err = pump(ctx, w, r, onBytes)
	closeErr := w.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = dst.Remove(it.Dst)
		return fmt.Errorf("%s: %w", path.Base(it.Src), err)
	}
	return nil
}

// pump copies with a cancellation check between blocks. The check is per block
// rather than per byte so cancelling is responsive without costing a syscall's
// worth of overhead on every byte.
func pump(ctx context.Context, w io.Writer, r io.Reader, onBytes func(int64)) error {
	buf := make([]byte, copyBuf)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, rerr := r.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			if onBytes != nil {
				onBytes(int64(n))
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// SameTree reports whether copying src into dstDir on the same filesystem would
// put a directory inside itself, which walks forever.
func SameTree(src FS, srcPath string, dst FS, dstDir string) bool {
	if src != dst {
		return false
	}
	return dstDir == srcPath || strings.HasPrefix(dstDir, srcPath+"/")
}
