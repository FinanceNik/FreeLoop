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
// internal/platform/windows starts nothing, but reaches the Credential Manager and the
// registry through golang.org/x/sys/windows, which can.
var execAllowed = []string{"internal/environment", "internal/platform/windows"}

// Direct file-system access is allowed in the environment layer and in the
// app's own infrastructure: the store, config loading and the platform layers.
// Everything else, the harnesses above all, goes through an Environment.
// cmd/freeloop-sbom is a tool make release runs on the build machine, to read the
// licenses of what went into the app; it is not part of the app.
var fsAllowed = []string{"internal/environment", "internal/store", "internal/config", "internal/platform/darwin", "internal/platform/linux", "internal/platform/windows", "cmd/freeloop-sbom"}

// Only the Wails adapter may depend on Wails; the engine stays
// transport-agnostic.
var wailsAllowed = []string{"cmd/freeloop"}

// macOS-only code lives in one package (NFR-XPLAT-002), and Linux's platform code, cgo
// included, in another, and Windows' in a third. Only cmd/freeloop, which picks one for
// the system it runs on, imports any of them.
var darwinAllowed = []string{"internal/platform/darwin"}
var linuxAllowed = []string{"internal/platform/linux"}
var platformImporters = []string{"cmd/freeloop"}

// The app reads its development switches, the FREELOOP_ variables, only through
// devEnv in cmd/freeloop, which a shipped build turns off. There every other read
// of the environment names a variable outside FREELOOP_ in a literal, so a new
// switch cannot get past devEnv. Elsewhere, only the packages listed here read a
// FREELOOP_ variable: the store's event check, which only `make test` and CI set.
const devEnvPkg, devEnvFunc = "cmd/freeloop", "devEnv"

var freeloopEnvAllowed = []string{"internal/store"}

// Packages whose import alone means process control or raw file access.
var execPackages = []string{"os/exec", "syscall", "golang.org/x/sys/unix", "golang.org/x/sys/windows"}
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
		case p == "C" && !in(darwinAllowed) && !in(linuxAllowed):
			report("%s: cgo outside %v and %v", rel, darwinAllowed, linuxAllowed)
		case strings.HasPrefix(p, "github.com/FinanceNik/Freeloop/internal/platform/") && !in(platformImporters):
			report("%s: imports %s; only %v may import the platform packages", rel, p, platformImporters)
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

	for _, decl := range f.Decls {
		for _, v := range envReads(pkg, decl, names, fset, in(freeloopEnvAllowed)) {
			report("%s:%s", rel, v)
		}
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

// envReads returns the reads of the environment in one declaration that get past
// devEnv, each as "line: what".
func envReads(pkg string, decl ast.Decl, names map[string]string, fset *token.FileSet, allowed bool) []string {
	if fn, ok := decl.(*ast.FuncDecl); ok && pkg == devEnvPkg && fn.Recv == nil && fn.Name.Name == devEnvFunc {
		return nil
	}
	isRead := func(n ast.Node) (*ast.SelectorExpr, bool) {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return nil, false
		}
		x, ok := sel.X.(*ast.Ident)
		return sel, ok && names[x.Name] == "os" && (sel.Sel.Name == "Getenv" || sel.Sel.Name == "LookupEnv")
	}
	var out []string
	called := map[*ast.SelectorExpr]bool{}
	ast.Inspect(decl, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			sel, ok := isRead(call.Fun)
			if !ok || len(call.Args) != 1 {
				return true
			}
			called[sel] = true
			line := fset.Position(call.Pos()).Line
			name, literal := "", false
			switch a := call.Args[0].(type) {
			case *ast.BasicLit:
				literal = a.Kind == token.STRING
				name, _ = strconv.Unquote(a.Value)
			case *ast.SelectorExpr:
				if a.Sel.Name == "HomeEnv" {
					name = "FREELOOP_HOME"
				}
			}
			switch {
			case pkg == devEnvPkg && (!literal || strings.HasPrefix(name, "FREELOOP_")):
				out = append(out, fmt.Sprintf("%d: os.%s outside %s; development switches go through it, which a shipped build turns off", line, sel.Sel.Name, devEnvFunc))
			case pkg != devEnvPkg && strings.HasPrefix(name, "FREELOOP_") && !allowed:
				out = append(out, fmt.Sprintf("%d: os.%s(%q); only %s reads development switches, through %s", line, sel.Sel.Name, name, devEnvPkg, devEnvFunc))
			}
			return true
		}
		if sel, ok := isRead(n); ok && !called[sel] && pkg == devEnvPkg {
			out = append(out, fmt.Sprintf("%d: os.%s used as a value outside %s", fset.Position(sel.Pos()).Line, sel.Sel.Name, devEnvFunc))
		}
		return true
	})
	return out
}

func TestDevSwitchReadsAreDetected(t *testing.T) {
	app := []byte(`package main

import (
	"os"

	"github.com/FinanceNik/Freeloop/internal/config"
)

func devEnv(name string) string { return devValue(false, name, os.Getenv) }

func f() {
	_ = os.Getenv("HOME")
	_ = os.Getenv("FREELOOP_DEV")
	_, _ = os.LookupEnv(config.HomeEnv)
	name := "FREELOOP_NEW"
	_ = os.Getenv(name)
	_ = devValue(true, "X", os.Getenv)
}
`)
	got := violations("cmd/freeloop/main.go", app)
	want := []string{":13: os.Getenv outside devEnv", ":14: os.LookupEnv outside devEnv", ":16: os.Getenv outside devEnv", ":17: os.Getenv used as a value"}
	if len(got) != len(want) {
		t.Fatalf("got %d violations, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i, w := range want {
		if !strings.Contains(got[i], w) {
			t.Errorf("violation %d = %q, want it to mention %q", i, got[i], w)
		}
	}
	engine := []byte("package engine\nimport \"os\"\nvar _ = os.Getenv(\"FREELOOP_DEV\")\nvar _ = os.Getenv(\"HOME\")\n")
	if v := violations("internal/engine/dev.go", engine); len(v) != 1 || !strings.Contains(v[0], `os.Getenv("FREELOOP_DEV")`) {
		t.Errorf("engine: %v", v)
	}
	store := []byte("package store\nimport \"os\"\nvar _ = os.Getenv(\"FREELOOP_CHECK_EVENTS\")\n")
	if v := violations("internal/store/check.go", store); len(v) != 0 {
		t.Errorf("store flagged: %v", v)
	}
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

func TestPlatformPackagesAreImportedOnlyByTheApp(t *testing.T) {
	src := []byte("package engine\nimport _ \"github.com/FinanceNik/Freeloop/internal/platform/linux\"\n")
	if v := violations("internal/engine/x.go", src); len(v) != 1 || !strings.Contains(v[0], "only [cmd/freeloop] may import the platform packages") {
		t.Errorf("engine importing the platform: %v", v)
	}
	if v := violations("cmd/freeloop/platform_linux.go", src); len(v) != 0 {
		t.Errorf("the app flagged: %v", v)
	}
	// The Linux platform layer reads files and may use cgo, like the macOS one.
	src = []byte("package linux\nimport \"C\"\nimport \"os\"\nvar _, _ = os.ReadFile(\"/proc/stat\")\n")
	if v := violations("internal/platform/linux/proc.go", src); len(v) != 0 {
		t.Errorf("platform/linux flagged: %v", v)
	}
	if v := violations("internal/engine/x.go", src); len(v) != 2 {
		t.Errorf("cgo and os.ReadFile outside the platform layers: %v", v)
	}
}
