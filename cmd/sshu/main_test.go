package main

import (
	"strings"
	"testing"
)

// The warning is the whole of what a dropped duplicate produces: nothing
// failed, sshu is running, and the only way the user learns the file and the
// list disagree is this line. So it has to NAME them — a count alone sends
// someone hunting through a file for which entry to remove.
func TestDupeWarningNamesWhatWasDropped(t *testing.T) {
	got := dupeWarning("hosts.yaml", []string{"prod", "web"})

	for _, want := range []string{"hosts.yaml", `"prod"`, `"web"`, "2"} {
		if !strings.Contains(got, want) {
			t.Errorf("the warning does not mention %s: %q", want, got)
		}
	}
	// And it says which one survived, because "a duplicate was ignored" leaves
	// open the question the user actually has: which of the two am I using?
	if !strings.Contains(got, "first") {
		t.Errorf("the warning should say the first of each name wins: %q", got)
	}
}

// A name repeated three times drops two entries and is named twice. That is not
// a bug in the message — it is how many extra copies are in the file, which is
// what the user has to go and delete.
func TestDupeWarningCountsEntriesNotNames(t *testing.T) {
	got := dupeWarning("hosts.yaml", []string{"prod", "prod"})
	if !strings.Contains(got, "2 duplicate entries") {
		t.Errorf("want two entries reported, got %q", got)
	}
}

// One is one. A message that says "1 duplicate entries" reads like a machine
// wrote it and nobody read it back.
func TestDupeWarningIsSingularForOne(t *testing.T) {
	got := dupeWarning("credentials.yaml", []string{"deploy"})
	if !strings.Contains(got, "1 duplicate entry") {
		t.Errorf("want the singular noun, got %q", got)
	}
	if strings.Contains(got, "entries") {
		t.Errorf("the plural leaked into the single case: %q", got)
	}
}
