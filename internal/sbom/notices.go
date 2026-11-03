package sbom

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"runtime/debug"
	"slices"
	"strings"
)

// Where the licenses of what is in a build are. Both come from the machine that
// built it, which holds every package the build used.
type NoticeSources struct {
	// ModCache is `go env GOMODCACHE`, NodeModules the frontend's node_modules.
	ModCache, NodeModules fs.FS
}

// Notices writes the license and notice files of everything in a build, the same
// things Build lists, for shipping beside the app. Missing lists what has no such
// file, which someone has to look at before a release: the text cannot say what
// is not there. Nothing depends on when it runs.
func Notices(app App, info *debug.BuildInfo, packageLock []byte, src NoticeSources) (text []byte, missing []string, err error) {
	var lock lockFile
	if err := json.Unmarshal(packageLock, &lock); err != nil {
		return nil, nil, fmt.Errorf("package-lock.json: %w", err)
	}
	type pkg struct {
		name, version string
		root          fs.FS
		dir           string
	}
	var pkgs []pkg
	for _, d := range info.Deps {
		if d.Replace != nil {
			d = d.Replace
		}
		pkgs = append(pkgs, pkg{d.Path, d.Version, src.ModCache, escapeModule(d.Path) + "@" + escapeModule(d.Version)})
	}
	for key, p := range lock.Packages {
		if !strings.HasPrefix(key, "node_modules/") || p.Dev || p.DevOpt || p.Link {
			continue
		}
		name := key[strings.LastIndex(key, "node_modules/")+len("node_modules/"):]
		// A nested copy is under its parent, not at the top.
		pkgs = append(pkgs, pkg{name, p.Version, src.NodeModules, strings.TrimPrefix(key, "node_modules/")})
	}
	slices.SortFunc(pkgs, func(a, b pkg) int {
		if c := strings.Compare(a.name, b.name); c != 0 {
			return c
		}
		return strings.Compare(a.version, b.version)
	})
	pkgs = slices.CompactFunc(pkgs, func(a, b pkg) bool { return a.name == b.name && a.version == b.version })

	var out bytes.Buffer
	fmt.Fprintf(&out, "%s %s includes the following software, under these licenses.\n", app.Name, app.Version)
	for _, p := range pkgs {
		files, err := licenseFiles(p.root, p.dir)
		if err != nil {
			return nil, nil, err
		}
		if len(files) == 0 {
			missing = append(missing, p.name+" "+p.version)
			continue
		}
		fmt.Fprintf(&out, "\n\n================================================================\n%s %s\n================================================================\n", p.name, p.version)
		for _, f := range files {
			b, err := fs.ReadFile(p.root, path.Join(p.dir, f))
			if err != nil {
				return nil, nil, err
			}
			out.WriteString("\n")
			out.Write(bytes.TrimSpace(b))
			out.WriteString("\n")
		}
	}
	return out.Bytes(), missing, nil
}

// licenseFiles names a package's license and notice files, in order.
func licenseFiles(root fs.FS, dir string) ([]string, error) {
	entries, err := fs.ReadDir(root, dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := strings.ToUpper(e.Name())
		if e.IsDir() {
			continue
		}
		for _, start := range []string{"LICENSE", "LICENCE", "COPYING", "NOTICE", "UNLICENSE"} {
			if strings.HasPrefix(name, start) {
				out = append(out, e.Name())
				break
			}
		}
	}
	slices.Sort(out)
	return out, nil
}

// escapeModule writes a module path or version as the module cache does: an upper
// case letter is ! and the letter in lower case.
func escapeModule(s string) string {
	var b strings.Builder
	for _, c := range s {
		if c >= 'A' && c <= 'Z' {
			b.WriteByte('!')
			c += 'a' - 'A'
		}
		b.WriteRune(c)
	}
	return b.String()
}
