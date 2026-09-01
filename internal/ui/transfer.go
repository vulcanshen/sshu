package ui

import (
	"context"
	"fmt"
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
		for _, it := range items {
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
// the ambient channel: always visible, never in the way (§7.2 — information
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
	return fmt.Sprintf("%s %d/%d · %d%%", glyphUpload, doneFiles, files, pct)
}

// ------------------------------------------------------------------- popup

// transfersPopup lists the jobs with their progress and lets one be cancelled.
// The summary answers "is anything happening"; this answers "what, and how far".
type transfersPopup struct {
	anim    popupAnimator
	cursor  int
	layer   int
	screenW int
	screenH int
}

func newTransfersPopup() transfersPopup {
	return transfersPopup{anim: newPopupAnimator("transfers")}
}

func (m transfersPopup) isActive() bool      { return m.anim.isActive() }
func (m transfersPopup) isInteractive() bool { return m.anim.isInteractive() }
func (m *transfersPopup) close() tea.Cmd     { return m.anim.close() }
func (m *transfersPopup) setSize(w, h int)   { m.screenW, m.screenH = w, h }

func (m *transfersPopup) open(layer int) tea.Cmd {
	m.layer, m.cursor = layer, 0
	return m.anim.open()
}

// update returns the index to cancel, or -1.
func (m *transfersPopup) update(msg tea.KeyMsg, n int) int {
	if !m.anim.isInteractive() {
		return -1
	}
	switch k := msg.String(); k {
	case "j", "down", "k", "up":
		m.cursor = moveCursor(m.cursor, n, k, n)
	case "c", "C":
		return m.cursor
	}
	return -1
}

func (m transfersPopup) view(jobs []*transferJob) string {
	innerW := popupInnerW(m.screenW, 58)
	dim := lipgloss.NewStyle().Foreground(dimColor)
	txt := lipgloss.NewStyle().Foreground(textColor)
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color(baseHex)).Background(handColor)

	var rows []string
	if len(jobs) == 0 {
		rows = []string{dim.Render(padRight("  nothing transferred yet", innerW))}
	}
	for i, j := range jobs {
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

	hint := hintLegend([][2]string{{"j/k", "move"}, {"c", "cancel"}, {"Esc", "close"}})
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
