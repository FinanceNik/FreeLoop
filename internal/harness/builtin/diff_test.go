package builtin

import (
	"strings"
	"testing"
)

func TestShortDiff(t *testing.T) {
	// Small changes stay whole.
	if a, b := shortDiff("a\nb\n", "a\nc\n"); a != "a\nb\n" || b != "a\nc\n" {
		t.Errorf("small: %q %q", a, b)
	}
	// One line of a 20 MB file stores that line and a few around it.
	line := strings.Repeat("x", 99) + "\n"
	before := strings.Repeat(line, 100000)
	after := strings.Repeat(line, 50000) + "changed\n" + strings.Repeat(line, 50000)
	a, b := shortDiff(before+before, after+before)
	if len(a)+len(b) > 4096 {
		t.Errorf("stored %d bytes for a one-line edit", len(a)+len(b))
	}
	if !strings.Contains(b, "changed\n") {
		t.Errorf("the change is gone: %q", b)
	}
	if strings.Count(b, "\n") != 2*diffContext+1 {
		t.Errorf("context: %q", b)
	}
	// Replacing everything is named, not stored.
	a, b = shortDiff(before, strings.ReplaceAll(before, "x", "y"))
	if a != "" || !strings.Contains(b, "too large") || len(b) > 200 {
		t.Errorf("huge: %d %q", len(a), b[:min(len(b), 100)])
	}
	// A new large file too.
	if a, b = shortDiff("", before); a != "" || !strings.Contains(b, "too large") {
		t.Errorf("new file: %d %d", len(a), len(b))
	}
	// Text with no final newline, and text the old side repeats, stay correct.
	head := strings.Repeat("k\n", 40000)
	a, b = shortDiff(head+"end", head+"end\nmore")
	if !strings.HasSuffix(a, "end") || !strings.HasSuffix(b, "end\nmore") {
		t.Errorf("ends: %q %q", a, b)
	}
}
