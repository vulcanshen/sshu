package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	terminu "github.com/vulcanshen/terminu-go"

	"github.com/vulcanshen/sshu/internal/remote"
)

// A transfer runs on its own goroutine and reports through atomics, so
// drawing a frame never waits on the network.

type jobState int32

const (
	jobRunning jobState = iota
	jobDone
	jobFailed
	jobCancelled
)

type job struct {
	id     int
	label  string // "3 files → db-replica:/var/backups"
	total  int64
	files  int
	cancel context.CancelFunc
	dst    *side

	done      atomic.Int64
	filesDone atomic.Int32
	state     atomic.Int32
	errText   atomic.Pointer[string]
	// cur is the destination path being written now: the one file on the far
	// side that exists and is not all there yet.
	cur atomic.Pointer[string]

	logged bool // its ending has reached the journals
}

func (j *job) status() jobState { return jobState(j.state.Load()) }

func (j *job) err() string {
	if p := j.errText.Load(); p != nil {
		return *p
	}
	return ""
}

// percent is by bytes, by files when there are no bytes to weigh.
func (j *job) percent() int {
	if j.total > 0 {
		return int(min(100, j.done.Load()*100/j.total))
	}
	if j.files == 0 {
		return 100
	}
	return int(min(100, int64(j.filesDone.Load())*100/int64(j.files)))
}

func (j *job) stateWord() string {
	switch j.status() {
	case jobDone:
		return "done"
	case jobCancelled:
		return "cancelled"
	case jobFailed:
		return "failed"
	}
	return "running"
}

func (t *transfer) running() int {
	n := 0
	for _, j := range t.jobs {
		if j.status() == jobRunning {
			n++
		}
	}
	return n
}

// progress is the running transfers' average, for the bar under the chips.
func (t *transfer) progress() (float64, bool) {
	n, pct := 0, 0
	for _, j := range t.jobs {
		if j.status() == jobRunning {
			n++
			pct += j.percent()
		}
	}
	if n == 0 {
		return 0, false
	}
	return float64(pct) / float64(n) / 100, true
}

// statusText is the statusbar's word on transfers: files and percent, the
// same width whatever the numbers (tdp L2).
func (t *transfer) statusText() string {
	frac, ok := t.progress()
	if !ok {
		return ""
	}
	files, done := 0, 0
	for _, j := range t.jobs {
		if j.status() == jobRunning {
			files += j.files
			done += int(j.filesDone.Load())
		}
	}
	// The value of the statusbar's "transfer: ", as wide however far it is.
	return terminu.Pad(fmt.Sprintf("%d/%d %d%%", done, files, int(frac*100)), len("9999/9999 100%"))
}

// receiving reports whether p, or something under it, is being written now.
func (t *transfer) receiving(p string) bool {
	for _, j := range t.jobs {
		if j.status() != jobRunning {
			continue
		}
		if c := j.cur.Load(); c != nil && (*c == p || strings.HasPrefix(*c, p+"/")) {
			return true
		}
	}
	return false
}

// arrivalFrame is the loading glyph a file still arriving wears in its row
// (tdp F7: how an item that is still coming in is shown is the app's), or
// "" with nothing arriving: the lib's loading icon, turning with the lib's.
func (t *transfer) arrivalFrame() string {
	if t.running() == 0 {
		return ""
	}
	return terminu.LoadingFrame() // the family's one turning icon
}

type xferTick struct{}

const xferEvery = 120 * time.Millisecond

func (t *transfer) tick() tea.Cmd {
	if t.ticking || t.running() == 0 {
		return nil
	}
	t.ticking = true
	return t.s.a.Tick(xferEvery, func(time.Time) tea.Msg { return xferTick{} })
}

// onXferTick redraws the arrivals and settles the jobs that ended: the
// destination is read again, the ending goes to the journals and a toast.
func (t *transfer) onXferTick() tea.Cmd {
	t.ticking = false
	var cmds []tea.Cmd
	for _, j := range t.jobs {
		if j.status() == jobRunning || j.logged {
			continue
		}
		j.logged = true
		j.dst.reload()
		switch j.status() {
		case jobDone:
			t.s.logChange("transfer done: " + j.label)
			cmds = append(cmds, t.s.a.Toast(terminu.Success, "Transferred "+plural(j.files, "file")))
		case jobCancelled:
			t.s.logChange("transfer cancelled: " + j.label)
			cmds = append(cmds, t.s.a.Toast(terminu.Info, "Transfer cancelled"))
		case jobFailed:
			t.s.logError("error", "Transfer failed: "+j.err(), j.label)
			cmds = append(cmds, t.s.a.Toast(terminu.Error, "Transfer failed: "+j.err()))
		}
	}
	for _, sd := range t.sides {
		sd.redraw()
	}
	t.s.refreshQuit()
	return tea.Batch(append(cmds, t.tick())...)
}

// queue plans the transfer of paths to the other side's directory in full
// first — how many files, which already exist at the far end — so an
// overwrite is asked once for the batch, never halfway through. It gives the
// question to ask, nil when nothing would be overwritten, and what to do
// then: an action's AskFor and Do (see asked).
func (sd *side) queue(paths []string) (*terminu.Ask, func() tea.Cmd) {
	t := sd.t
	dst := t.other(sd)
	a := t.s.a
	say := func(cmd func() tea.Cmd) (*terminu.Ask, func() tea.Cmd) { return nil, cmd }
	if dst.fs == nil {
		return say(func() tea.Cmd { return a.Toast(terminu.Error, "The other side has no host yet") })
	}
	for _, p := range paths {
		if remote.SameTree(sd.fs, p, dst.fs, dst.cwd) {
			return say(func() tea.Cmd { return a.Toast(terminu.Error, "That would copy a directory into itself") })
		}
	}
	items, total, err := remote.Plan(sd.fs, paths, dst.cwd)
	if err != nil {
		return say(func() tea.Cmd { return a.Error("Transfer failed", err.Error()) })
	}
	files := 0
	for _, it := range items {
		if !it.IsDir {
			files++
		}
	}
	if files == 0 {
		return say(func() tea.Cmd { return a.Toast(terminu.Info, "Nothing to transfer") })
	}
	label := fmt.Sprintf("%s → %s:%s", plural(files, "file"), dst.host, foldHomePath(dst.cwd, dst.home))
	start := func() tea.Cmd { return t.startJob(sd, dst, items, total, files, label) }
	if n := len(remote.Conflicts(dst.fs, items)); n > 0 {
		return &terminu.Ask{Title: "Overwrite", Subject: dst.host,
			Details: []string{fmt.Sprintf("%s, %d will overwrite.", plural(files, "file"), n)}, Warn: []int{0},
			Question: "Transfer to " + dst.host + ":" + dst.cwd + "?", Verb: "transfer"}, start
	}
	return nil, start
}

func (t *transfer) startJob(src, dst *side, items []remote.Item, total int64, files int, label string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	t.nextID++
	j := &job{id: t.nextID, label: label, total: total, files: files, cancel: cancel, dst: dst}
	t.jobs = append(t.jobs, j)
	sfs, dfs := src.fs, dst.fs
	go func() {
		defer j.cur.Store(nil)
		for _, it := range items {
			p := it.Dst
			j.cur.Store(&p)
			if err := remote.CopyItem(ctx, sfs, dfs, it, func(n int64) { j.done.Add(n) }); err != nil {
				s := err.Error()
				j.errText.Store(&s)
				if ctx.Err() != nil {
					j.state.Store(int32(jobCancelled))
				} else {
					j.state.Store(int32(jobFailed))
				}
				return
			}
			if !it.IsDir {
				j.filesDone.Add(1)
			}
		}
		j.state.Store(int32(jobDone))
	}()
	t.s.refreshQuit()
	return t.tick()
}

// jobsMenu lists the transfers of this run, newest first, each row following
// its transfer while the menu is open (Action.Live). Enter on a running one
// asks, with its whole text, whether to cancel it (Enter:cancel); on one that
// ended it shows the whole of it (components/dialog/menu, work lists).
func (t *transfer) jobsMenu() tea.Cmd {
	if len(t.jobs) == 0 {
		return t.s.a.Toast(terminu.Info, "Nothing transferred yet")
	}
	var acts []terminu.Action
	for i := len(t.jobs) - 1; i >= 0; i-- {
		j := t.jobs[i]
		// As wide whatever it says: the row keeps the width it opened with.
		desc := func() string {
			return fmt.Sprintf("%-9s · %3d%% · %*d/%d files", j.stateWord(), j.percent(),
				len(strconv.Itoa(j.files)), j.filesDone.Load(), j.files)
		}
		act := terminu.Action{Label: j.label, Desc: desc(), Live: func() (string, string) { return j.label, desc() }}
		if j.status() == jobRunning {
			act.EnterHint, act.Stay = "cancel", true
		} else {
			act.Detail = true
		}
		act.Do = func() tea.Cmd {
			if j.status() != jobRunning {
				return t.s.a.Viewer(newViewer("Transfer", j.label, glyphJobs, jobLines(j)))
			}
			return t.s.a.Confirm(terminu.Ask{Title: "Cancel", Subject: j.label, Details: jobLines(j),
				Question: "Cancel this transfer?", Verb: "cancel transfer"},
				func() tea.Cmd { j.cancel(); return nil })
		}
		acts = append(acts, act)
	}
	return t.s.a.Menu("Jobs", terminu.MenuSection{Actions: acts})
}

func jobLines(j *job) []string {
	lines := []string{j.label, "", fmt.Sprintf("%s · %d%% · %d/%d files · %s", j.stateWord(), j.percent(),
		j.filesDone.Load(), j.files, humanSize(j.total))}
	if e := j.err(); e != "" {
		lines = append(append(lines, ""), strings.Split(e, "\n")...)
	}
	return lines
}

// cancelAll stops every running transfer, on the way out.
func (t *transfer) cancelAll() {
	for _, j := range t.jobs {
		if j.status() == jobRunning {
			j.cancel()
		}
	}
}
