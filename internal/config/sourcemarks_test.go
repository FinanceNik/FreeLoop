package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceMarksKeepWhatTheyCanBe(t *testing.T) {
	in := map[string]SourceMark{
		"good":    {Slot: 3, Letters: "Ab"},
		"unicode": {Letters: "\u00c9Z"},
		"slot":    {Slot: 9, Letters: "OK"},
		"letters": {Slot: 2, Letters: "<script>"},
		"both":    {Slot: -1, Letters: "a b"},
	}
	out, issues := CleanSourceMarks(in)
	if out["good"] != (SourceMark{Slot: 3, Letters: "Ab"}) || out["unicode"].Letters != "\u00c9Z" {
		t.Errorf("good marks changed: %+v", out)
	}
	if out["slot"] != (SourceMark{Letters: "OK"}) || out["letters"] != (SourceMark{Slot: 2}) {
		t.Errorf("the good part of a bad mark was not kept: %+v", out)
	}
	if _, ok := out["both"]; ok {
		t.Errorf("an empty mark was kept: %+v", out["both"])
	}
	if len(issues) != 4 {
		t.Errorf("issues = %+v", issues)
	}
	if none, issues := CleanSourceMarks(nil); len(none) != 0 || issues != nil {
		t.Errorf("none = %v, %v", none, issues)
	}
}

func TestLoadDropsABadSourceMarkAndReportsIt(t *testing.T) {
	dir := t.TempDir()
	file := "version: 1\nui:\n  theme: dark\n  sourceMarks:\n    a: {slot: 2, letters: Hi}\n    b: {slot: 99}\n"
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, issues, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UI.SourceMarks["a"] != (SourceMark{Slot: 2, Letters: "Hi"}) {
		t.Errorf("marks = %+v", cfg.UI.SourceMarks)
	}
	if _, ok := cfg.UI.SourceMarks["b"]; ok {
		t.Error("the bad mark stayed")
	}
	found := false
	for _, i := range issues {
		found = found || strings.Contains(i.Field, "sourceMarks.b")
	}
	if !found {
		t.Errorf("not reported: %+v", issues)
	}
}
