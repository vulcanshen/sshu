package ui

import (
	"strings"
	"testing"
)

// The logo grid must be a clean rectangle — render indexes it as rows×cols, so
// a ragged row would panic or misplace pixels.
func TestSplashLogoIsRectangular(t *testing.T) {
	want := len(logoPixels[0])
	for r, row := range logoPixels {
		if len(row) != want {
			t.Errorf("row %d width %d, want %d", r, len(row), want)
		}
	}
	if want != 25 {
		t.Errorf("logo is %d cols wide, want 25 (from icon.svg)", want)
	}
}

func TestSplashInitialState(t *testing.T) {
	m := newSplashModel()
	if m.isActive() {
		t.Error("a new splash must be inactive")
	}
	if out := m.render(80, 40); out != "" {
		t.Error("an inactive splash must render empty")
	}
}

func TestSplashShow(t *testing.T) {
	m := newSplashModel()
	cmd := m.show()
	if cmd == nil {
		t.Fatal("show() must return the first animation tick")
	}
	if !m.isActive() {
		t.Error("show() must activate the splash")
	}
	if len(m.pixelOrder) == 0 {
		t.Error("show() must populate pixelOrder")
	}
	if out := m.render(80, 40); out == "" {
		t.Error("an active splash must render non-empty")
	}
}

func TestSplashTickDoesNotExceedTotal(t *testing.T) {
	m := newSplashModel()
	m.show()
	for i := 0; i < 4000 && m.revealedCount < len(m.pixelOrder); i++ {
		m, _ = m.update(splashTickMsg{})
	}
	if m.revealedCount != len(m.pixelOrder) {
		t.Errorf("revealedCount = %d, want %d (all pixels)", m.revealedCount, len(m.pixelOrder))
	}
	// Every cell of the grid must end up painted — the bg pass alone covers it.
	if got := len(m.pixelOrder); got < len(logoPixels)*len(logoPixels[0]) {
		t.Errorf("pixelOrder has %d entries, want at least the full sheet %d",
			got, len(logoPixels)*len(logoPixels[0]))
	}
}

// The byline arrives with the hint — the splash's last reveal — as TWO lines
// ("developed by" over the address), sitting ABOVE the Esc hint.
func TestSplashRevealsTheByline(t *testing.T) {
	m := newSplashModel()
	m.show()
	if out := m.render(80, 40); strings.Contains(out, "vulcan.shen.2304@gmail.com") {
		t.Error("the byline must not be visible before the hint stage")
	}
	m, _ = m.update(splashHintMsg{})
	out := m.render(80, 40)
	if !strings.Contains(out, "developed by") || !strings.Contains(out, "vulcan.shen.2304@gmail.com") {
		t.Error("the hint stage should reveal both byline lines")
	}
	if strings.Contains(out, "developed by vulcan") {
		t.Error("the address must be its own line, not appended to the label")
	}
	if strings.Index(out, "vulcan.shen.2304@gmail.com") > strings.Index(out, "Press Esc to close") {
		t.Error("the byline must sit above the Esc hint")
	}
}

// V outside a pty reveals the logo; any key puts the frame back; V inside a
// pty belongs to the remote and must not open it.
func TestSplashOpensOnVAndAnyKeyCloses(t *testing.T) {
	m := appWith(sample(), nil)
	m = pressA(m, "V")
	if !m.splash.isActive() {
		t.Fatal("V should reveal the easter egg")
	}
	// The first ticks paint the sheet; only then is there a glyph to see.
	for i := 0; i < 3; i++ {
		next, _ := m.Update(splashTickMsg{})
		m = next.(AppModel)
	}
	if !strings.Contains(m.View(), pixelGlyph) {
		t.Error("the active splash should replace the frame with the logo")
	}
	m = pressA(m, "j")
	if m.splash.isActive() {
		t.Error("any key should dismiss the splash")
	}
	if strings.Contains(m.View(), pixelGlyph) {
		t.Error("the frame should be back after dismissing")
	}

	pty := openOne(t)
	pty = pressA(pty, "V")
	if pty.splash.isActive() {
		t.Error("inside a pty V belongs to the remote, not to the easter egg")
	}
}
