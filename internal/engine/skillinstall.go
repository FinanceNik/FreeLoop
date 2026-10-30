package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/environment"
	"github.com/FinanceNik/Freeloop/internal/skill"
)

// Importing and installing skills (SKL-003, SKL-005) copies them into the
// app's skill store. The source, ~/.claude/skills included, is only read,
// through an environment confined to it; nothing is ever written back.

// Limits on what one skill may bring.
const (
	maxSkillFiles = 500
	maxSkillBytes = 20 << 20
)

// SkillCandidate is a skill a source offers, as the preview shows it.
type SkillCandidate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	// Status is new, same (already installed from identical files),
	// changed, or invalid.
	Status string   `json:"status"`
	Issue  string   `json:"issue,omitempty"`
	Files  []string `json:"files"`
	// Scripts are the files under scripts/; they run only through the
	// shell tool, which asks for each command.
	Scripts []string `json:"scripts"`
	// Changes, for a changed skill, lists files as "+ added", "- removed"
	// and "~ modified" against the installed copy.
	Changes []string `json:"changes"`
	// Installed is set when a skill of this name is in the store.
	Installed *config.SkillProvenance `json:"installed,omitempty"`
	// Diff shows the text changes of a changed skill against the
	// installed copy, file by file (SKL-006).
	Diff string `json:"diff,omitempty"`
}

type bundle struct {
	cand  SkillCandidate
	files map[string][]byte
	// git is set for a skill read from a git repository (SKL-006).
	git *gitSource
}

// memFS serves a bundle's files to skill.Parse.
type memFS map[string][]byte

func (m memFS) ReadFile(p string) ([]byte, error) {
	if b, ok := m[path.Clean(p)]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("%s: not found", p)
}

func (m memFS) ReadDir(p string) ([]environment.FileInfo, error) {
	p = path.Clean(p)
	seen := map[string]bool{}
	var out []environment.FileInfo
	for name := range m {
		rel := name
		if p != "." {
			var ok bool
			if rel, ok = strings.CutPrefix(name, p+"/"); !ok {
				continue
			}
		}
		first, rest, dir := strings.Cut(rel, "/")
		if !seen[first] {
			seen[first] = true
			out = append(out, environment.FileInfo{Name: first, Path: path.Join(p, first), IsDir: dir && rest != ""})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ClaudeSkillsDir is where Claude Code keeps the user's skills.
func (e *Engine) ClaudeSkillsDir() string {
	return filepath.Join(e.home, ".claude", "skills")
}

// readOnlyEnv returns an environment that may read inside root only.
func readOnlyEnv(root string) (*environment.Local, error) {
	root = environment.Canonical(root)
	return environment.NewLocal(environment.LocalOptions{ID: "source", Root: root, Guard: insideGuard{root}})
}

// readSource reads the skills src offers: a skill folder, a folder of
// skills, or a .zip of either.
func (e *Engine) readSource(src string) ([]bundle, error) {
	src = strings.TrimSpace(src)
	if g, ok := parseGitSource(src); ok {
		return e.readGit(g)
	}
	if strings.HasPrefix(src, "~/") {
		src = filepath.Join(e.home, src[2:])
	}
	if !filepath.IsAbs(src) {
		return nil, fmt.Errorf("%q is not an absolute path", src)
	}
	files := map[string][]byte{}
	if strings.EqualFold(filepath.Ext(src), ".zip") {
		env, err := readOnlyEnv(filepath.Dir(src))
		if err != nil {
			return nil, err
		}
		b, err := env.FS().ReadFile(filepath.Base(src))
		if err != nil {
			return nil, err
		}
		if files, err = unzip(b); err != nil {
			return nil, err
		}
	} else {
		if !environment.IsDir(src) {
			return nil, fmt.Errorf("there is no skill folder or .zip at %s", src)
		}
		env, err := readOnlyEnv(src)
		if err != nil {
			return nil, err
		}
		if err := collect(env.FS(), ".", files); err != nil {
			return nil, err
		}
	}
	return group(files, src)
}

// collect reads every file below dir, skipping dot files and folders.
func collect(fs skill.FS, dir string, into map[string][]byte) error {
	entries, err := fs.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, en := range entries {
		if strings.HasPrefix(en.Name, ".") {
			continue
		}
		p := path.Join(dir, en.Name)
		if en.IsDir {
			if err := collect(fs, p, into); err != nil {
				return err
			}
			continue
		}
		if len(into) >= maxSkillFiles*4 {
			return errors.New("the source has too many files")
		}
		b, err := fs.ReadFile(p)
		if err != nil {
			continue // a special file or a link out of the source
		}
		into[p] = b
	}
	return nil
}

// unzip reads a zip's regular files, refusing paths that leave the archive.
func unzip(b []byte) (map[string][]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("not a zip file: %w", err)
	}
	out := map[string][]byte{}
	total := int64(0)
	for _, f := range r.File {
		name := path.Clean(strings.ReplaceAll(f.Name, "\\", "/"))
		if f.FileInfo().IsDir() || strings.HasPrefix(path.Base(name), ".") || strings.HasPrefix(name, "__MACOSX/") {
			continue
		}
		if !f.Mode().IsRegular() {
			continue // links are not followed
		}
		if !filepath.IsLocal(name) {
			return nil, fmt.Errorf("the zip holds %q, which leaves the archive", f.Name)
		}
		total += int64(f.UncompressedSize64)
		if total > maxSkillBytes*4 || len(out) >= maxSkillFiles*4 {
			return nil, errors.New("the zip is too large")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(rc, maxSkillBytes+1))
		rc.Close()
		if err != nil {
			return nil, err
		}
		out[name] = data
	}
	return out, nil
}

// group splits files into skills: SKILL.md at the top is one skill named
// by its frontmatter (or the source's name); otherwise each top-level
// folder with a SKILL.md is one.
func group(files map[string][]byte, src string) ([]bundle, error) {
	split := map[string]map[string][]byte{}
	if _, ok := files[skill.FileName]; ok {
		front, _, err := skill.Split(files[skill.FileName])
		name := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
		if err == nil {
			var fm struct{ Name string }
			if yamlName(front, &fm.Name) && fm.Name != "" {
				name = fm.Name
			}
		}
		split[name] = files
	} else {
		for p, b := range files {
			top, rest, ok := strings.Cut(p, "/")
			if !ok {
				continue
			}
			if _, has := files[top+"/"+skill.FileName]; !has {
				continue
			}
			if split[top] == nil {
				split[top] = map[string][]byte{}
			}
			split[top][rest] = b
		}
	}
	if len(split) == 0 {
		return nil, errors.New("no SKILL.md found; a skill is a folder with a SKILL.md")
	}
	var out []bundle
	for name, fs := range split {
		c := SkillCandidate{Name: name, Source: src, Status: "new", Files: []string{}, Scripts: []string{}, Changes: []string{}}
		for p := range fs {
			if p != skill.FileName {
				c.Files = append(c.Files, p)
			}
			if strings.HasPrefix(p, "scripts/") {
				c.Scripts = append(c.Scripts, p)
			}
		}
		slices.Sort(c.Files)
		slices.Sort(c.Scripts)
		size := 0
		for _, b := range fs {
			size += len(b)
		}
		mem := memFS{}
		for p, b := range fs {
			mem[path.Join(name, p)] = b
		}
		s, err := skill.Parse(mem, name, skill.SourceApp, "")
		switch {
		case err != nil:
			c.Status, c.Issue = "invalid", err.Error()
		case len(fs) > maxSkillFiles:
			c.Status, c.Issue = "invalid", fmt.Sprintf("%d files; at most %d", len(fs), maxSkillFiles)
		case size > maxSkillBytes:
			c.Status, c.Issue = "invalid", fmt.Sprintf("%d MB; at most %d", size>>20, maxSkillBytes>>20)
		default:
			c.Description = s.Description
		}
		out = append(out, bundle{cand: c, files: fs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].cand.Name < out[j].cand.Name })
	return out, nil
}

// yamlName reads the name field of a frontmatter block.
func yamlName(front []byte, into *string) bool {
	for _, line := range strings.Split(string(front), "\n") {
		if v, ok := strings.CutPrefix(line, "name:"); ok {
			*into = strings.Trim(strings.TrimSpace(v), `"'`)
			return true
		}
	}
	return false
}

func digest(files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for p := range files {
		names = append(names, p)
	}
	slices.Sort(names)
	h := sha256.New()
	for _, p := range names {
		sum := sha256.Sum256(files[p])
		fmt.Fprintf(h, "%s\x00%x\n", p, sum)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// compare fills a candidate's status against the installed copy.
func (e *Engine) compare(b *bundle) {
	if b.cand.Status == "invalid" {
		return
	}
	prov, ok := config.ReadSkillProvenance(e.dataDir, b.cand.Name)
	fs, root := e.appSkillsFS()
	installed := map[string][]byte{}
	if fs != nil {
		collect(fs, b.cand.Name, installed)
	}
	if len(installed) == 0 {
		return
	}
	if ok {
		b.cand.Installed = &prov
	} else {
		b.cand.Installed = &config.SkillProvenance{Source: filepath.Join(root, b.cand.Name)}
	}
	have := map[string][]byte{}
	for p, data := range installed {
		have[strings.TrimPrefix(p, b.cand.Name+"/")] = data
	}
	if digest(have) == digest(b.files) {
		b.cand.Status = "same"
		return
	}
	b.cand.Status = "changed"
	for p, data := range b.files {
		old, ok := have[p]
		switch {
		case !ok:
			b.cand.Changes = append(b.cand.Changes, "+ "+p)
		case !bytes.Equal(old, data):
			b.cand.Changes = append(b.cand.Changes, "~ "+p)
			if len(b.cand.Diff) < maxSkillDiff && bytes.IndexByte(old, 0) < 0 && bytes.IndexByte(data, 0) < 0 {
				b.cand.Diff += renderDiff(p, string(old), string(data)) + "\n"
			}
		}
	}
	for p := range have {
		if _, ok := b.files[p]; !ok {
			b.cand.Changes = append(b.cand.Changes, "- "+p)
		}
	}
	slices.SortFunc(b.cand.Changes, func(x, y string) int { return strings.Compare(x[2:], y[2:]) })
}

// PreviewSkills lists the skills src offers and how each compares with the
// store; nothing is written.
func (e *Engine) PreviewSkills(ctx context.Context, src string) ([]SkillCandidate, error) {
	bundles, err := e.readSource(src)
	if err != nil {
		return nil, err
	}
	out := make([]SkillCandidate, 0, len(bundles))
	for i := range bundles {
		e.compare(&bundles[i])
		out = append(out, bundles[i].cand)
	}
	return out, nil
}

// InstallSkills copies the named skills from src into the store with their
// provenance, replacing changed ones; skills already identical are left as
// they are. It returns the names written.
func (e *Engine) InstallSkills(ctx context.Context, src string, names []string) ([]string, error) {
	bundles, err := e.readSource(src)
	if err != nil {
		return nil, err
	}
	var done []string
	for i := range bundles {
		b := &bundles[i]
		if !slices.Contains(names, b.cand.Name) {
			continue
		}
		e.compare(b)
		switch b.cand.Status {
		case "invalid":
			return done, fmt.Errorf("%s: %s", b.cand.Name, b.cand.Issue)
		case "same":
			continue
		}
		prov := config.SkillProvenance{Source: b.cand.Source, ImportedAt: time.Now().UTC(), Digest: digest(b.files)}
		if b.git != nil {
			prov.Source, prov.Ref, prov.Commit = b.git.URL, b.git.Ref, b.git.Commit
		}
		if err := config.InstallSkill(e.dataDir, b.cand.Name, b.files, prov); err != nil {
			return done, err
		}
		done = append(done, b.cand.Name)
	}
	e.forgetSkills()
	return done, nil
}

// RemoveSkill deletes a skill from the store; project skills are the
// project's and are never touched.
func (e *Engine) RemoveSkill(name string) error {
	if err := config.RemoveSkill(e.dataDir, name); err != nil {
		return err
	}
	e.forgetSkills()
	return nil
}
