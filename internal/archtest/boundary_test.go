// Package archtest enforces the architecture's import boundaries. It runs
// with `make test` and `make lint`.
package archtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const root = "../.."

// Process launches are allowed only in the environment layer (ENV-001).
var execAllowed = []string{"internal/environment"}

// Direct file-system access is allowed in the environment layer and in the
// app's own infrastructure: the store, config loading and the platform layer.
// Everything else, the harnesses above all, goes through an Environment.
var fsAllowed = []string{"internal/environment", "internal/store", "internal/config", "internal/platform/darwin"}

// Only the Wails adapter may depend on Wails; the engine stays
// transport-agnostic.
var wailsAllowed = []string{"cmd/freeloop"}

// macOS-only code lives in one package (NFR-XPLAT-002).
var darwinAllowed = []string{"internal/platform/darwin"}

// Packages whose import alone means process control or raw file access.
var execPackages = []string{"os/exec", "syscall", "golang.org/x/sys/unix"}
var fsPackages = []string{"io/ioutil"}

// Functions in package os and path/filepath that touch the file system or
// start processes.
var osFSFuncs = []string{
	"Chdir", "Chmod", "Chown", "Chtimes", "CopyFS", "Create", "CreateTemp", "DirFS",
	"Lchown", "Link", "Lstat", "Mkdir", "MkdirAll", "MkdirTemp", "Open", "OpenFile",
	"OpenInRoot", "OpenRoot", "ReadDir", "ReadFile", "Readlink", "Remove", "RemoveAll",
	"Rename", "Stat", "Symlink", "Truncate", "WriteFile",
}
var osExecFuncs = []string{"StartProcess", "FindProcess"}
var filepathFSFuncs = []string{"Walk", "WalkDir", "Glob", "EvalSymlinks"}

func TestBoundaries(t *testing.T) {
	var checked int
	for _, top := range []string{"internal", "cmd", "frontend"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "testdata") {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			checked++
			rel, _ := filepath.Rel(root, path)
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, v := range violations(filepath.ToSlash(rel), src) {
				t.Error(v)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked == 0 {
		t.Fatal("no Go files checked; is root wrong?")
	}
}

// violations returns the boundary violations in one Go source file; rel is
// its slash-separated path from the repository root.
func violations(rel string, src []byte) []string {
	var out []string
	report := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rel, src, parser.ParseComments)
	if err != nil {
		return []string{err.Error()}
	}
	pkg := filepath.ToSlash(filepath.Dir(rel))
	in := func(allowed []string) bool {
		return slices.ContainsFunc(allowed, func(a string) bool { return pkg == a || strings.HasPrefix(pkg, a+"/") })
	}

	if !in(darwinAllowed) {
		if strings.HasSuffix(rel, "_darwin.go") {
			report("%s: macOS-only file outside %v", rel, darwinAllowed)
		}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if strings.HasPrefix(c.Text, "//go:build") && strings.Contains(c.Text, "darwin") {
					report("%s: darwin build constraint outside %v", rel, darwinAllowed)
				}
			}
		}
	}

	names := map[string]string{} // local name -> import path, for os and filepath
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		switch {
		case p == "C" && !in(darwinAllowed):
			report("%s: cgo outside %v", rel, darwinAllowed)
		case slices.Contains(execPackages, p) && !in(execAllowed):
			report("%s: imports %s; only %v may start processes (ENV-001)", rel, p, execAllowed)
		case slices.Contains(fsPackages, p) && !in(fsAllowed):
			report("%s: imports %s; file access goes through internal/environment (ENV-001)", rel, p)
		case strings.HasPrefix(p, "github.com/wailsapp/wails") && !in(wailsAllowed):
			report("%s: imports %s; only %v may depend on Wails", rel, p, wailsAllowed)
		case p == "os" || p == "path/filepath":
			name := filepath.Base(p)
			if imp.Name != nil {
				name = imp.Name.Name
			}
			names[name] = p
		}
	}
	if len(names) == 0 {
		return out
	}

	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		pos := fset.Position(sel.Pos())
		switch names[x.Name] {
		case "os":
			if slices.Contains(osExecFuncs, sel.Sel.Name) && !in(execAllowed) {
				report("%s:%d: os.%s; only %v may start processes (ENV-001)", rel, pos.Line, sel.Sel.Name, execAllowed)
			}
			if slices.Contains(osFSFuncs, sel.Sel.Name) && !in(fsAllowed) {
				report("%s:%d: os.%s; file access goes through internal/environment (ENV-001)", rel, pos.Line, sel.Sel.Name)
			}
		case "path/filepath":
			if slices.Contains(filepathFSFuncs, sel.Sel.Name) && !in(fsAllowed) {
				report("%s:%d: filepath.%s; file access goes through internal/environment (ENV-001)", rel, pos.Line, sel.Sel.Name)
			}
		}
		return true
	})
	return out
}

func TestViolationsAreDetected(t *testing.T) {
	src := []byte(`//go:build darwin

package builtin

import (
	"os"
	"os/exec"
	fp "path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var _ = exec.Command
var _ application.Options

func f() {
	os.ReadFile("x")
	fp.WalkDir(".", nil)
	_ = os.Getenv("HOME")
}
`)
	got := violations("internal/harness/builtin/tools.go", src)
	want := []string{"darwin build constraint", "imports os/exec", "imports github.com/wailsapp", "os.ReadFile", "filepath.WalkDir"}
	if len(got) != len(want) {
		t.Fatalf("got %d violations, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i, w := range want {
		if !strings.Contains(got[i], w) {
			t.Errorf("violation %d = %q, want it to mention %q", i, got[i], w)
		}
	}
	if v := violations("internal/environment/local.go", []byte("package environment\nimport \"os/exec\"\nvar _ = exec.Command\n")); len(v) != 0 {
		t.Errorf("environment flagged: %v", v)
	}
}
