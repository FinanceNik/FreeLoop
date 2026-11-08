//go:build unix

package environment

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// swapProject is a project with a folder d and a secret folder beside it, and
// functions that swap d for a link to the secret one and back.
func swapProject(t *testing.T) (l *Local, root, secret string, swap, restore func()) {
	t.Helper()
	base, _ := filepath.EvalSymlinks(t.TempDir())
	root, secret = filepath.Join(base, "project"), filepath.Join(base, "secret")
	for _, d := range []string{filepath.Join(root, "d"), secret} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	l, err := NewLocal(LocalOptions{Root: root, Guard: rootOnlyGuard{root}})
	if err != nil {
		t.Fatal(err)
	}
	d, aside := filepath.Join(root, "d"), filepath.Join(root, "d-aside")
	swap = func() {
		if os.Rename(d, aside) == nil {
			os.Symlink(secret, d)
		}
	}
	restore = func() {
		if _, err := os.Lstat(aside); err != nil {
			return
		}
		// A write in the moment d was missing may have made it again.
		os.RemoveAll(d)
		os.Rename(aside, d)
	}
	return l, root, secret, swap, restore
}

func secretEntries(t *testing.T, secret string) []string {
	t.Helper()
	es, err := os.ReadDir(secret)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

// A folder swapped for a link between the check and making the missing folders
// must not have them made through it, outside the project. Before the fix,
// MkdirAll followed the link (security review, 2026-11-03: the file tools' race
// on creating folders).
func TestMakingFoldersDoesNotFollowALinkAfterTheCheck(t *testing.T) {
	t.Cleanup(func() { afterCheck, beforeMkdir = func() {}, func() {} })

	// Swapped after the check, before any folder is made.
	l, _, secret, swap, restore := swapProject(t)
	afterCheck = func() { afterCheck = func() {}; swap() }
	if err := l.FS().WriteFile("d/new/deep/file", []byte("x")); err == nil {
		t.Error("wrote through a folder swapped after the check")
	}
	if names := secretEntries(t, secret); len(names) != 0 {
		t.Errorf("made %v outside the project", names)
	}
	restore()

	// Swapped as the first missing folder is about to be made. What has been
	// opened is the folder that was checked, which is where the rest is made.
	beforeMkdir = func() { beforeMkdir = func() {}; swap() }
	l.FS().WriteFile("d/new2/deep/file", []byte("x"))
	if names := secretEntries(t, secret); len(names) != 0 {
		t.Errorf("made %v outside the project", names)
	}
	restore()

	// Swapped before a deeper missing folder is made.
	n := 0
	beforeMkdir = func() {
		if n++; n == 2 {
			beforeMkdir = func() {}
			swap()
		}
	}
	l.FS().WriteFile("d/new3/deep/file", []byte("x"))
	if names := secretEntries(t, secret); len(names) != 0 {
		t.Errorf("made %v outside the project", names)
	}
	restore()

	// Ordinary use is unchanged.
	if err := l.FS().WriteFile("a/b/c/file", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if b, err := l.FS().ReadFile("a/b/c/file"); err != nil || string(b) != "x" {
		t.Errorf("read back %q, %v", b, err)
	}
	if err := l.FS().WriteFile("a/b", []byte("x")); err == nil {
		t.Error("wrote over a folder")
	}
	if err := l.FS().WriteFile("a/b/c/file/below", []byte("x")); err == nil {
		t.Error("made a folder under a file")
	}
}

// The same race run freely, as a sandboxed command would: one goroutine swaps the
// folder for a link and back as fast as it can while the file tool writes into new
// folders under it. The review measured one win in 8,543 tries; this loop wins
// far more often against the code before the fix, about one write in 60 and one
// in 4 with -race, so 5,000 tries (2,000 with -short) always catch it.
func TestMakingFoldersRacingASwap(t *testing.T) {
	tries := 5000
	if testing.Short() {
		tries = 2000
	}
	l, root, secret, swap, restore := swapProject(t)
	var stop atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			swap()
			restore()
		}
	}()
	written := 0
	for i := range tries {
		if l.FS().WriteFile(fmt.Sprintf("d/n%d/f", i), []byte("x")) == nil {
			written++
		}
	}
	stop.Store(true)
	wg.Wait()
	if names := secretEntries(t, secret); len(names) != 0 {
		t.Fatalf("%d of %d writes made folders outside the project: %v", len(names), tries, names[:min(len(names), 5)])
	}
	if written == 0 {
		t.Error("no write succeeded")
	}
	// What was written is in the project.
	es, _ := os.ReadDir(filepath.Join(root, "d"))
	if len(es) == 0 {
		t.Error("nothing written into the project")
	}
	t.Logf("%d of %d writes succeeded, none outside the project", written, tries)
}
