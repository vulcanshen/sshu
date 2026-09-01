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
