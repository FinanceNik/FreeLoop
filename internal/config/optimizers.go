package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"unicode"

	"go.yaml.in/yaml/v3"
)

// OptimizersDir holds one YAML file per token optimizer (HRN-019).
const OptimizersDir = "optimizers"

// OptimizerOff, as a session's or profile's choice, keeps a session from
// taking the optimizer it would otherwise inherit.
const OptimizerOff = "off"

// Optimizer is a program that rewrites shell commands so their output costs
// fewer tokens (HRN-019). It runs after the broker has approved the original
// command and has no say in what may run.
type Optimizer struct {
	ID   string `yaml:"-"` // the file name without .yaml
	Name string `yaml:"name"`
	// Program is looked up the way the session's commands find it. A rewrite
	// may only put this word in front of a command.
	Program string `yaml:"program"`
	// Rewrite are the arguments before the command to rewrite. A run that
	// exits with one of RewriteExits printed the new command; any other exit
	// leaves the command as it is.
	Rewrite      []string `yaml:"rewrite"`
	RewriteExits []int    `yaml:"rewriteExits"`
	// ProbeCommand must rewrite to ProbeWant, which tells this program from
	// another with the same name.
	ProbeCommand string `yaml:"probeCommand"`
	ProbeWant    string `yaml:"probeWant"`
	// Version are the arguments that print its version.
	Version []string `yaml:"version,omitempty"`
	// Rewrites maps the programs it may rewrite to the program's word for each:
	// git to git, cat to read. Nothing else is asked about or accepted. Which
	// tool runs must stay the same: for tsc or gradle, RTK prefers a copy in the
	// repository, so the approved command would have run the repository's code.
	Rewrites map[string]string `yaml:"rewrites"`
	// LeaveAlone are options that keep a command from being rewritten, because
	// the program changes what they do. An entry matches the word itself and
	// the word followed by =.
	LeaveAlone []string `yaml:"leaveAlone,omitempty"`
	// LeaveAloneCommands are commands it shortens in a way the reader cannot
	// tell, whose words appear in this order: git log matches git -C x log too.
	LeaveAloneCommands []string `yaml:"leaveAloneCommands,omitempty"`
	// Env is added to every command while the optimizer is on, and DataEnv
	// points variables at files in the session's own folder. Their names start
	// with the program's name, so they cannot change how anything else runs.
	Env     map[string]string `yaml:"env,omitempty"`
	DataEnv map[string]string `yaml:"dataEnv,omitempty"`
	// Install tells the user how to get the program. It is typed into a
	// terminal for them to read and run.
	Install string `yaml:"install,omitempty"`
	// Download is what the app can fetch for the user; a file cannot name one.
	Download *OptimizerDownload `yaml:"-"`
	// Gain are the arguments that print what it has saved, as JSON a built-in one's
	// report is known to have. A file cannot name them.
	Gain []string `yaml:"-"`
	// Source is the file it came from; empty for a built-in one.
	Source string `yaml:"-"`
}

// OptimizerDownload is a program the app can install itself, pinned by checksum.
type OptimizerDownload struct {
	Version string
	License string
	// Assets are .tar.gz builds, or .zip ones for Windows, by Go's names for the
	// system and the architecture, as "linux/amd64" (AssetKey).
	Assets map[string]OptimizerAsset
}

// OptimizerAsset is one build.
type OptimizerAsset struct {
	URL    string
	SHA256 string
}

// BuiltinOptimizers are the optimizers every install offers.
func BuiltinOptimizers() []Optimizer {
	return []Optimizer{{
		ID: "rtk", Name: "RTK", Program: "rtk",
		Rewrite: []string{"rewrite"}, RewriteExits: []int{0, 3},
		ProbeCommand: "git status", ProbeWant: "rtk git status",
		Version: []string{"--version"},
		Gain:    []string{"gain", "--format", "json"},
		// Tried raw and rewritten in identical repositories, with copies of each
		// planted in the repository. Others, such as gh and curl, await the same.
		Rewrites: map[string]string{
			"git": "git", "ls": "ls", "cat": "read", "head": "read", "tail": "read", "grep": "grep", "rg": "rg",
			"find": "find", "wc": "wc", "diff": "diff", "go": "go", "cargo": "cargo",
		},
		// RTK writes its own summary to the file: 114 bytes for a 1975-byte
		// diff.
		LeaveAlone: []string{"--output", "--output-file"},
		// RTK shows 10 of 31 commits and says nothing.
		LeaveAloneCommands: []string{"git log"},
		Env: map[string]string{
			"RTK_TELEMETRY_DISABLED":    "1",
			"RTK_ONRAMP_DISABLED":       "1",
			"RTK_SUPPRESS_HOOK_WARNING": "1",
		},
		// Full outputs are kept here, for `rtk recall`, so they go when the
		// session goes.
		DataEnv: map[string]string{"RTK_DB_PATH": "tracking.db", "RTK_RECALL_DB": "recall.db"},
		Install: rtkInstall(),
		// The edition without RTK's BUSL-licensed engine. Hashes from the release's
		// checksums.txt.
		Download: &OptimizerDownload{Version: "1.1.0", License: "Apache-2.0", Assets: map[string]OptimizerAsset{
			"darwin/arm64": {URL: "https://github.com/rtk-ai/rtk/releases/download/v1.1.0/rtk-oss-aarch64-apple-darwin.tar.gz", SHA256: "1623590dd8d65374f61589c599c764c08ba9ce993ebe6b601312a89a3c874558"},
			"darwin/amd64": {URL: "https://github.com/rtk-ai/rtk/releases/download/v1.1.0/rtk-oss-x86_64-apple-darwin.tar.gz", SHA256: "559156238dd3e148c84c88c3a62a7594b874f68793208994e1b02e77a9d9abf9"},
			"linux/arm64":  {URL: "https://github.com/rtk-ai/rtk/releases/download/v1.1.0/rtk-oss-aarch64-unknown-linux-gnu.tar.gz", SHA256: "57a4fac13d18da77f0577ea8b68def7934b934bf76d0a6e161f0bfd06b1e1078"},
			"linux/amd64":  {URL: "https://github.com/rtk-ai/rtk/releases/download/v1.1.0/rtk-oss-x86_64-unknown-linux-musl.tar.gz", SHA256: "4dec94a64035e08189ae82fd5408fe4ae9ae27a665fc07e3e3ec3c4c2d8e8a3e"},
			// A .zip holding rtk.exe (NFR-XPLAT-003); there is no Windows build for arm64.
			"windows/amd64": {URL: "https://github.com/rtk-ai/rtk/releases/download/v1.1.0/rtk-oss-x86_64-pc-windows-msvc.zip", SHA256: "9b076379be9d86011ca3f3a0264d0be4851da5535453a47f67fa46b4e893fbb2"},
		}},
	}}
}

var programName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// Validate reports settings that cannot work.
func (o Optimizer) Validate() error {
	switch {
	case !idPattern.MatchString(o.ID):
		return fmt.Errorf("id %q must be lowercase letters, digits and dashes", o.ID)
	case o.ID == OptimizerOff:
		return fmt.Errorf("%q means no optimizer, so it cannot name one", OptimizerOff)
	case strings.TrimSpace(o.Name) == "":
		return errors.New("name is required")
	case !programName.MatchString(o.Program):
		return fmt.Errorf("program %q must be a program's name, without a path", o.Program)
	case len(o.Rewrite) == 0:
		return errors.New("rewrite: the arguments that ask for a rewrite are required")
	case len(o.RewriteExits) == 0:
		return errors.New("rewriteExits: the exit codes of a rewrite are required")
	case len(o.Rewrites) == 0:
		return errors.New("rewrites: the programs it may rewrite are required")
	case strings.TrimSpace(o.ProbeCommand) == "":
		return errors.New("probeCommand: a command to try it with is required")
	case !strings.HasPrefix(o.ProbeWant, o.Program+" "):
		return fmt.Errorf("probeWant must be what probeCommand becomes, starting with %s", o.Program)
	case strings.ContainsFunc(o.Install, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }):
		// A newline would run it the moment it is typed into a terminal, and an
		// invisible character could make it read as something else.
		return errors.New("install must be one line of plain text")
	}
	for from, to := range o.Rewrites {
		if !programName.MatchString(from) || !programName.MatchString(to) {
			return fmt.Errorf("rewrites: %s: %s must be program names, without a path", from, to)
		}
	}
	prefix := o.varPrefix()
	for k := range o.Env {
		if !varName.MatchString(k) || !strings.HasPrefix(k, prefix) {
			return fmt.Errorf("env: %q must be a variable name starting with %s", k, prefix)
		}
	}
	for k, file := range o.DataEnv {
		if !varName.MatchString(k) || !strings.HasPrefix(k, prefix) {
			return fmt.Errorf("dataEnv: %q must be a variable name starting with %s", k, prefix)
		}
		if file == "" || file == "." || file == ".." || file != filepath.Base(file) {
			return fmt.Errorf("dataEnv: %s must name a file, without a path", k)
		}
	}
	return nil
}

// varPrefix is what the names of its variables start with: RTK_ for rtk. PATH,
// ZDOTDIR, DYLD_INSERT_LIBRARIES or GIT_SSH_COMMAND would reach every command.
func (o Optimizer) varPrefix() string {
	return strings.ToUpper(regexp.MustCompile(`[^A-Za-z0-9]`).ReplaceAllString(o.Program, "_")) + "_"
}

// Fingerprint identifies what the optimizer runs. A file's optimizer runs only
// after the user approved exactly this (HRN-019).
func (o Optimizer) Fingerprint() string {
	o.Source, o.Download = "", nil
	b, err := json.Marshal(o)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// LoadOptimizers returns the built-in optimizers with those in optimizers/ on
// top, sorted by ID. Files with problems are skipped and reported.
func LoadOptimizers(dir string) ([]Optimizer, []Issue) {
	out := BuiltinOptimizers()
	var issues []Issue
	odir := filepath.Join(dir, OptimizersDir)
	entries, err := os.ReadDir(odir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		issues = append(issues, Issue{File: odir, Message: err.Error()})
	}
	for _, de := range entries {
		id, ok := strings.CutSuffix(de.Name(), ".yaml")
		if !ok || de.IsDir() {
			continue
		}
		path := filepath.Join(odir, de.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			issues = append(issues, Issue{File: path, Message: err.Error()})
			continue
		}
		var o Optimizer
		dec := yaml.NewDecoder(bytes.NewReader(b))
		dec.KnownFields(true)
		if err := dec.Decode(&o); err != nil && !errors.Is(err, io.EOF) {
			issues = append(issues, Issue{File: path, Message: err.Error()})
			continue
		}
		o.ID, o.Source = id, filepath.Join(OptimizersDir, de.Name())
		if err := o.Validate(); err != nil {
			issues = append(issues, Issue{File: path, Message: err.Error()})
			continue
		}
		out = slices.DeleteFunc(out, func(x Optimizer) bool { return x.ID == id })
		out = append(out, o)
	}
	slices.SortFunc(out, func(a, b Optimizer) int { return strings.Compare(a.ID, b.ID) })
	return out, issues
}

// rtkInstall is how to get RTK by hand: Homebrew's formula on a Mac. Nothing
// is offered elsewhere, because "brew install" is not what the user has there.
func rtkInstall() string {
	if runtime.GOOS == "darwin" {
		return "brew install rtk"
	}
	return ""
}

// AssetKey names this system's build among an optimizer's downloads.
func AssetKey() string { return runtime.GOOS + "/" + runtime.GOARCH }
