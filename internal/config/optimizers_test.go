package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeOptimizer(t *testing.T, dir, id, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, OptimizersDir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, OptimizersDir, id+".yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

const goodOptimizer = `name: Short
program: short
rewrite: [rewrite]
rewriteExits: [0]
probeCommand: git status
probeWant: short git status
install: brew install short
`

// HRN-019: a file adds an optimizer or replaces a built-in one; a bad file is
// reported and skipped.
func TestLoadOptimizers(t *testing.T) {
	dir := t.TempDir()
	if list, issues := LoadOptimizers(dir); len(issues) != 0 || len(list) != 1 || list[0].ID != "rtk" {
		t.Fatalf("no files: %+v %+v", list, issues)
	}
	writeOptimizer(t, dir, "short", goodOptimizer)
	writeOptimizer(t, dir, "rtk", strings.Replace(strings.Replace(goodOptimizer, "program: short", "program: rtk", 1), "short git status", "rtk git status", 1))
	writeOptimizer(t, dir, "typo", goodOptimizer+"unknownField: 1\n")
	list, issues := LoadOptimizers(dir)
	if len(issues) != 1 || !strings.Contains(issues[0].File, "typo") {
		t.Errorf("issues = %+v", issues)
	}
	if len(list) != 2 || list[0].ID != "rtk" || list[0].Name != "Short" || list[0].Source == "" || list[1].ID != "short" {
		t.Errorf("list = %+v", list)
	}
}

func TestOptimizerValidate(t *testing.T) {
	good := func() Optimizer {
		return Optimizer{ID: "x", Name: "X", Program: "x", Rewrite: []string{"r"}, RewriteExits: []int{0}, ProbeCommand: "git status", ProbeWant: "x git status"}
	}
	if err := good().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Optimizer){
		"the off word":      func(o *Optimizer) { o.ID = OptimizerOff },
		"no name":           func(o *Optimizer) { o.Name = " " },
		"a path":            func(o *Optimizer) { o.Program = "/tmp/x" },
		"a shell word":      func(o *Optimizer) { o.Program = "x;y" },
		"no rewrite":        func(o *Optimizer) { o.Rewrite = nil },
		"no exits":          func(o *Optimizer) { o.RewriteExits = nil },
		"no probe":          func(o *Optimizer) { o.ProbeCommand = "" },
		"another program":   func(o *Optimizer) { o.ProbeWant = "y git status" },
		"a runnable line":   func(o *Optimizer) { o.Install = "brew install x\nrm -rf ~" },
		"a bad variable":    func(o *Optimizer) { o.Env = map[string]string{"A B": "1"} },
		"a path in dataEnv": func(o *Optimizer) { o.DataEnv = map[string]string{"A": "../../x"} },
	} {
		o := good()
		change(&o)
		if o.Validate() == nil {
			t.Errorf("%s passed", name)
		}
	}
}
