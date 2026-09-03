package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/vulcanshen/sshu/internal/store"
)

// The credential picker must be VISIBLE over the form, not merely active —
// it shipped once painted underneath it, and every isActive assertion stayed
// green while the user saw nothing happen. Only a rendered frame catches a
// z-order bug.
func TestCredPickerRendersAboveTheForm(t *testing.T) {
	creds := []store.Credential{{Name: "ops", User: "root", Auth: store.AuthPassword, Password: "x"}}
	m := pressA(credApp(sample(), creds), "A")
	m.form.fields[fAuth].sel = 2 // credential
	m.form.focus = fCredential
	m = pressA(m, "enter")

	if !m.credPicker.isActive() {
		t.Fatal("setup: the picker should be active")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "use credential") || !strings.Contains(view, "ops") {
		t.Fatalf("the picker must be painted over the form:\n%s", view)
	}
}

// An invalid credential submit refuses loudly — a toast on top of the marked
// field — and the form stays up so the user can finish.
func TestCredFormInvalidSubmitToastsAndStays(t *testing.T) {
	m := pressA(appWith(nil, nil), "1", "j", "enter", "A")
	if !m.credFormUI.isActive() {
		t.Fatal("setup: no form")
	}
	m.credFormUI.focus = cName
	m = pressA(m, "enter") // nothing filled in

	if !m.credFormUI.isActive() {
		t.Fatal("refusing must not close the form")
	}
	if !m.toast.isActive() {
		t.Fatal("the refusal must raise a toast")
	}
	if m.credFormUI.err == "" || m.credFormUI.errIdx != cName {
		t.Fatalf("the error row must mark the field too, err=%q idx=%d",
			m.credFormUI.err, m.credFormUI.errIdx)
	}
}

// The layout strip's custom label reads rows × columns — the same order the
// prompt asks in.
func TestCustomLabelReadsRowsThenColumns(t *testing.T) {
	m := twoOnGrid(t)
	m.ssh.layout = layoutCustom
	m.ssh.gridR, m.ssh.gridC = 3, 2 // the strip label reads rows × columns
	m.ssh.setFocus(panelSessions)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "custom 3×2") {
		t.Fatalf("the strip should read rows × columns (3×2):\n%s", view)
	}
}

// runningJob is a transfer frozen mid-flight at done/total bytes.
func runningJob(id int, done, total int64) *transferJob {
	j := &transferJob{id: id, label: "job", total: total, files: 1, cancel: func() {}}
	j.done.Store(done)
	return j
}

// progress blends only the RUNNING jobs; a finished one stops weighing in,
// which is also what returns the rule to a plain line at the end.
func TestProgressBlendsRunningJobsOnly(t *testing.T) {
	var m transferModel
	if _, moving := m.progress(); moving {
		t.Fatal("nothing running, nothing moving")
	}
	m.jobs = append(m.jobs, runningJob(1, 30, 100), runningJob(2, 50, 100))
	finished := runningJob(3, 100, 100)
	finished.state.Store(int32(xferDone))
	m.jobs = append(m.jobs, finished)

	if pct, moving := m.progress(); !moving || pct != 40 {
		t.Fatalf("pct=%d moving=%v, want the running pair's blend 40", pct, moving)
	}
}

// greenRun counts the rule cells inked liveColor: the run opened by the green
// SGR, up to the next escape.
func greenRun(row, green string) int {
	at := strings.Index(row, green)
	if at < 0 {
		return 0
	}
	seg := row[at:]
	if m := strings.Index(seg, "m"); m >= 0 {
		seg = seg[m+1:]
	}
	if e := strings.Index(seg, "\x1b"); e >= 0 {
		seg = seg[:e]
	}
	return strings.Count(seg, "─")
}

// While bytes move, the rule under the tab row doubles as a progress bar:
// liveColor ink from the left for the blended percent, borderDim past it —
// on EVERY tab — and a plain line again the moment the transfer ends.
func TestTheRuleTurnsGreenWithTheTransfer(t *testing.T) {
	withColour(t)
	m := sized(sample(), 100, 26)
	m.transfers.jobs = append(m.transfers.jobs, runningJob(1, 50, 100))
	green := ansiOf(t, liveColor)

	for _, key := range []string{"M", "S"} {
		rule := strings.Split(pressA(m, key).View(), "\n")[1]
		if got := greenRun(rule, green); got != 50 {
			t.Errorf("%s: green run is %d cells, want 50 of 100", key, got)
		}
	}

	m.transfers.jobs[0].state.Store(int32(xferDone))
	if strings.Contains(strings.Split(m.View(), "\n")[1], green) {
		t.Error("the rule must return to plain when nothing is moving")
	}
}

// The transfer summary is an action in FLIGHT, not a resting fact — §7.2,
// information arriving is not dimmed: it reports in liveColor. Statuses that
// merely describe state stay dim.
func TestTransferStatusIsGreenNotDim(t *testing.T) {
	withColour(t)
	m := pressA(sized(sample(), 100, 26), "F")
	m.transfers.jobs = append(m.transfers.jobs, runningJob(1, 50, 100))
	green := ansiOf(t, liveColor)

	if row := strings.Split(m.View(), "\n")[0]; !strings.Contains(row, green) {
		t.Error("the running summary should be liveColor")
	}
	if row := strings.Split(pressA(m, "S").View(), "\n")[0]; strings.Contains(row, green) {
		t.Error("a resting status must stay dim; green belongs to the moving one")
	}
}

// The summary is the ambient "something is happening" channel, so it has to
// LOOK like it is happening. A percentage can sit at 99% for a long time on a
// big file, and a number that does not move is what stuck looks like — the
// dots turn on every tick, the way the dial's spinner does.
func TestTheTransferSummarySpins(t *testing.T) {
	m := pressA(sized(sample(), 100, 26), "F")
	m.transfers.jobs = append(m.transfers.jobs, runningJob(1, 50, 100))

	first := m.transfers.summary()
	if !strings.HasPrefix(first, spinnerFrames[0]) {
		t.Fatalf("the summary should lead with the spinner, got %q", first)
	}
	if !strings.Contains(first, glyphUpload) {
		t.Errorf("the transfer glyph still says which kind of work it is: %q", first)
	}

	// One tick of the loop that already repaints it must move the frame on.
	next, _ := m.Update(xferTickMsg{})
	m = next.(AppModel)
	if got := m.transfers.summary(); got == first {
		t.Errorf("the spinner did not advance on a tick: %q", got)
	}

	// And it stops with the transfer — a spinner over a finished job would say
	// work is happening when none is.
	m.transfers.jobs[0].state.Store(int32(xferDone))
	if got := m.transfers.summary(); got != "" {
		t.Errorf("nothing is moving, so the summary should be empty, got %q", got)
	}
}
