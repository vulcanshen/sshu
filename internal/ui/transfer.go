package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vulcanshen/sshu/internal/remote"
)

// A transfer runs on its own goroutine and reports progress through atomics.
// The render path only reads them, so drawing a frame never waits on a copy —
// which matters, because a copy blocks on the network for as long as it likes.
type xferState int32

const (
	xferRunning xferState = iota
	xferDone
	xferFailed
	xferCancelled
)

type transferJob struct {
	id    int
	label string // "3 items -> db-replica:/var/backups"
	total int64
	files int

	done      atomic.Int64
	filesDone atomic.Int32
	state     atomic.Int32
	errText   atomic.Pointer[string]

	cancel context.CancelFunc

	// cur is the DESTINATION path being written this instant, nil between
	// items. It is the one file in the job that exists on the far side and is
	// not all there yet — the rest either do not exist yet or are finished.
	cur atomic.Pointer[string]

	// logged: this job's ending has reached the app log. Touched only on the
	// UI loop (logFinishedTransfers), never by the copy goroutine.
	logged bool
}

func (j *transferJob) status() xferState { return xferState(j.state.Load()) }

func (j *transferJob) err() string {
	if p := j.errText.Load(); p != nil {
		return *p
	}
	return ""
}

// percent is bytes-based, falling back to files when there is nothing to weigh
// (a batch of empty files, or of directories).
func (j *transferJob) percent() int {
	if j.total > 0 {
		return int(min(100, j.done.Load()*100/j.total))
	}
	if j.files == 0 {
		return 100
	}
	return int(min(100, int64(j.filesDone.Load())*100/int64(j.files)))
}

type transferModel struct {
	jobs   []*transferJob
	nextID int
	// spinAt is the summary spinner's frame. Like the dial's, it counts ticks
	// rather than reading the clock, so the animation does not depend on when
	// a frame happens to be drawn.
	spinAt int
}

// xferTickMsg repaints while anything is moving.
type xferTickMsg struct{}

// xferDoneMsg retires a job.
type xferDoneMsg struct{ id int }

const xferTickEvery = 120 * time.Millisecond

// runningCount is how many jobs are still moving bytes.
func (m transferModel) runningCount() int {
	n := 0
	for _, j := range m.jobs {
		if j.status() == xferRunning {
			n++
		}
	}
	return n
}

func (m transferModel) anyRunning() bool {
	for _, j := range m.jobs {
		if j.status() == xferRunning {
			return true
		}
	}
	return false
}

func (m transferModel) tick() tea.Cmd {
	if !m.anyRunning() {
		return nil
	}
	return tea.Tick(xferTickEvery, func(time.Time) tea.Msg { return xferTickMsg{} })
}

// start launches a job. The plan is already made, so the total is known from the
// first frame — a progress bar that discovers its own denominator halfway
// through is worse than none.
func (m *transferModel) start(src, dst remote.FS, items []remote.Item, total int64, label string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	j := &transferJob{
		id: m.nextID + 1, label: label, total: total, cancel: cancel,
	}
	for _, it := range items {
		if !it.IsDir {
			j.files++
		}
	}
	m.nextID++
	m.jobs = append(m.jobs, j)

	go func() {
		defer j.cur.Store(nil)
		for _, it := range items {
			dstPath := it.Dst
			j.cur.Store(&dstPath)
			if err := remote.CopyItem(ctx, src, dst, it, func(n int64) {
				j.done.Add(n)
			}); err != nil {
				s := err.Error()
				j.errText.Store(&s)
				if ctx.Err() != nil {
					j.state.Store(int32(xferCancelled))
				} else {
					j.state.Store(int32(xferFailed))
				}
				return
			}
			if !it.IsDir {
				j.filesDone.Add(1)
			}
		}
		j.state.Store(int32(xferDone))
	}()

	id := j.id
	return tea.Batch(m.tick(), func() tea.Msg {
		// A done message is only used to refresh the listing; the state itself
		// is read from the atomics, so a missed message costs nothing.
		return xferDoneMsg{id: id}
	})
}

func (m *transferModel) cancelAll() {
	for _, j := range m.jobs {
		if j.status() == xferRunning {
			j.cancel()
		}
	}
}

func (m *transferModel) cancelJob(i int) {
	if i >= 0 && i < len(m.jobs) && m.jobs[i].status() == xferRunning {
		m.jobs[i].cancel()
	}
}

// progress is the running jobs' blended percent — the one number the two
// ambient channels (the summary in the status slot, the rule under the tab
// row) both read, so they can never disagree.
func (m transferModel) progress() (pct int, moving bool) {
	var n int
	for _, j := range m.jobs {
		if j.status() != xferRunning {
			continue
		}
		n++
		pct += j.percent()
	}
	if n == 0 {
		return 0, false
	}
	return pct / n, true
}

// summary is the one line the tab row carries while anything is moving. It is
// the ambient channel: always visible, never in the way (tdp T2 — information
// arriving is not dimmed).
func (m transferModel) summary() string {
	pct, moving := m.progress()
	if !moving {
		return ""
	}
	var files, doneFiles int
	for _, j := range m.jobs {
		if j.status() != xferRunning {
			continue
		}
		files += j.files
		doneFiles += int(j.filesDone.Load())
	}
	// The spinner leads, the way the dial's does: a percentage can sit at 99%
	// for a while on a big file, and a number that is not moving is what
	// "stuck" looks like. The turning dots are the answer to that, and the
	// transfer glyph behind them still says WHICH kind of work it is.
	return fmt.Sprintf("%s %s %d/%d · %d%%",
		spinnerFrames[m.spinAt%len(spinnerFrames)], glyphUpload, doneFiles, files, pct)
}

// arrivals is what is landing in the panels this instant: the destination
// paths being written, and the spinner frame to draw beside them. The sftp
// view is HANDED this rather than reaching into the transfer engine for it —
// the same arrangement as status(), which is passed its summary.
type arrivals struct {
	paths []string
	frame string
}

// receiving reports whether the row at p is being written into right now.
// Either it IS the file arriving, or it is a directory the file is arriving
// INSIDE — and the second case is the common one: transfer a directory and the
// row you can see is the directory, not the file being written three levels
// down inside it.
func (a arrivals) receiving(p string) bool {
	if p == "" {
		return false
	}
	for _, dst := range a.paths {
		if dst == p || strings.HasPrefix(dst, p+"/") {
			return true
		}
	}
	return false
}

func (m transferModel) arrivals() arrivals {
	var paths []string
	for _, j := range m.jobs {
		if j.status() != xferRunning {
			continue
		}
		if p := j.cur.Load(); p != nil {
			paths = append(paths, *p)
		}
	}
	if len(paths) == 0 {
		return arrivals{}
	}
	return arrivals{paths: paths, frame: spinnerFrames[m.spinAt%len(spinnerFrames)]}
}

// ------------------------------------------------------------------- popup

// transfersPopup lists the jobs with their progress — a menu (tdp F1): Enter
// opens the job under the cursor in full, c cancels it. The summary answers "is
// anything happening"; this answers "what, and how far", and Enter answers
// "why" for a job that failed — the bar has room for the start of the reason,
// not the end, where the part that says why usually is.
type transfersPopup struct {
	anim    popupAnimator
	cursor  int
	top     int // first job on screen, when there are more than fit
	shown   int // jobs on screen when it opened: its height from then on (tdp F7)
	layer   int
	screenW int
	screenH int
}

// jobsOpen is what a keystroke asks of the Jobs popup.
type jobsOpen int

const (
	jobsNothing jobsOpen = iota
	jobsCancel
	jobsDetail
)

func newTransfersPopup() transfersPopup {
	return transfersPopup{anim: newPopupAnimator("transfers")}
}

func (m transfersPopup) isActive() bool      { return m.anim.isActive() }
func (m transfersPopup) isInteractive() bool { return m.anim.isInteractive() }
func (m *transfersPopup) close() tea.Cmd     { return m.anim.close() }
func (m *transfersPopup) setSize(w, h int)   { m.screenW, m.screenH = w, h }

func (m *transfersPopup) open(layer, n int) tea.Cmd {
	m.layer, m.cursor, m.top, m.shown = layer, 0, 0, n
	return m.anim.open()
}

// visible is how many jobs the box shows: the ones there were when it opened,
// or as many as the screen holds at two rows each — beyond that it scrolls
// with the cursor (tdp F7).
func (m transfersPopup) visible() int {
	return max(1, min(m.shown, popupBudget(m.screenH)/2))
}

// update reports what the key asked for, about job m.cursor.
func (m *transfersPopup) update(msg tea.KeyMsg, n int) jobsOpen {
	if !m.anim.isInteractive() {
		return jobsNothing
	}
	switch k := msg.String(); k {
	case "j", "down", "k", "up":
		m.cursor = moveCursor(m.cursor, n, k, n)
		m.top = scrollTop(m.top, m.cursor, m.visible(), n)
	case "enter":
		if m.cursor < n {
			return jobsDetail
		}
	case "c", "C":
		if m.cursor < n {
			return jobsCancel
		}
	}
	return jobsNothing
}

// jobLines is the Enter view of one job: what it moved, how far it got, and —
// the reason to open it — the whole of whatever stopped it.
func jobLines(j *transferJob, w int) []string {
	state := "running"
	switch j.status() {
	case xferDone:
		state = "done"
	case xferCancelled:
		state = "cancelled"
	case xferFailed:
		state = "failed"
	}
	lines := []string{
		" " + j.label,
		"",
		fmt.Sprintf(" %s · %d%% · %d/%d files · %s", state, j.percent(),
			j.filesDone.Load(), j.files, humanSize(j.total)),
	}
	if e := j.err(); e != "" {
		lines = append(lines, "")
		for _, para := range strings.Split(e, "\n") {
			for _, l := range wrapPlain(para, max(8, w)) {
				lines = append(lines, " "+l)
			}
		}
	}
	return lines
}

func (m transfersPopup) view(jobs []*transferJob) string {
	innerW := popupInnerW(m.screenW)
	dim := lipgloss.NewStyle().Foreground(dimColor)
	txt := lipgloss.NewStyle().Foreground(textColor)
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(handColor)

	var rows []string
	if len(jobs) == 0 {
		rows = []string{dim.Render(padRight("  nothing transferred yet", innerW))}
	}
	vis := m.visible()
	top := scrollTop(m.top, m.cursor, vis, len(jobs))
	for i := top; i < min(len(jobs), top+vis); i++ {
		j := jobs[i]
		style := txt
		switch j.status() {
		case xferDone:
			style = lipgloss.NewStyle().Foreground(liveColor)
		case xferFailed, xferCancelled:
			style = lipgloss.NewStyle().Foreground(warnColor)
		}
		line := "  " + j.label
		if i == m.cursor {
			rows = append(rows, bar.Render(padRight(line, innerW)))
		} else {
			rows = append(rows, style.Render(padRight(line, innerW)))
		}
		rows = append(rows, dim.Render(padRight("  "+progressBar(j, innerW-4), innerW)))
	}
	if m.shown > 0 {
		rows = fillRows(rows, 2*vis, innerW)
	}

	// c is offered only while the job under the cursor can still be cancelled:
	// a hint lists what works now (tdp M6), and ? lists the rest, dimmed.
	pairs := [][2]string{{"j/k", "move"}, {"Enter", "open"}, {"c", "cancel"}, {"Esc", "close"}}
	if m.cursor < len(jobs) && jobs[m.cursor].status() != xferRunning {
		pairs = slices.Delete(pairs, 2, 3)
	}
	if len(jobs) == 0 {
		pairs = [][2]string{{"Esc", "close"}}
	}
	if len(jobs) > vis {
		pairs = append([][2]string{{"", itoa(m.cursor+1) + " of " + itoa(len(jobs))}}, pairs...)
	}
	hint := pairs
	return drawPopupBox(popupLayerColor(m.layer), " "+glyphUpload+" Transfers ", hint,
		animRows(m.anim, capRows(rows, m.screenH)), innerW)
}

// progressBar is a filled run plus the numbers behind it. The bar is for
// glancing, the numbers are for knowing.
func progressBar(j *transferJob, w int) string {
	note := fmt.Sprintf("%d%%  %d/%d", j.percent(), j.filesDone.Load(), j.files)
	switch j.status() {
	case xferDone:
		note = "done  " + plural(j.files, "file")
	case xferCancelled:
		note = "cancelled"
	case xferFailed:
		note = "failed: " + j.err()
	}

	barW := max(0, w-dispW(note)-2)
	if barW < 4 {
		return truncate(note, max(0, w))
	}
	filled := barW * j.percent() / 100
	return strings.Repeat("━", filled) + strings.Repeat("╌", barW-filled) + "  " + note
}
