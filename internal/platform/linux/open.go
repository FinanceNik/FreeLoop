//go:build linux

package linux

import (
	"os"
	"path/filepath"
	"strings"
)

// xdgOpen is xdg-open by its usual absolute path, so a PATH that a project's
// environment shaped cannot put another program in its place; where it is elsewhere, by name.
func xdgOpen() string {
	if fi, err := os.Stat("/usr/bin/xdg-open"); err == nil && !fi.IsDir() {
		return "/usr/bin/xdg-open"
	}
	return "xdg-open"
}

// runnable says whether opening path with whatever the desktop chose could run it
// instead of showing it: xdg-open hands a .desktop file, an AppImage, a Java archive or
// an executable file to its handler, which may launch it, and a package to an installer. Anything that is not a plain file, or that
// cannot be looked at, counts as runnable, so it is only ever revealed.
func runnable(path string) bool {
	for _, p := range []string{path, resolve(path)} {
		switch strings.ToLower(filepath.Ext(p)) {
		case ".desktop", ".appimage", ".jar", ".deb", ".rpm", ".flatpak", ".flatpakref":
			return true
		}
	}
	fi, err := os.Stat(path) // follows a link: a file named like text may be a link to a program
	if err != nil || !fi.Mode().IsRegular() {
		return true
	}
	return fi.Mode().Perm()&0o111 != 0
}

// resolve is the file a link leads to, or path where it is not one.
func resolve(path string) string {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		return p
	}
	return path
}

// reveal is the command that shows path's folder, which holds the file without running it.
func reveal(path string) []string {
	return []string{xdgOpen(), filepath.Dir(path)}
}

// absolute makes path absolute, so a name that begins with a dash is never an option.
func absolute(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// EditorArgv is the command that opens a file in the user's editor. xdg-open goes by the
// file's type and could launch what it is given, so only a plain file that nothing
// would run is opened; any other is revealed in its folder. A file the desktop would
// not hand to an editor is shown wrongly, never run.
func EditorArgv(path string) []string {
	path = absolute(path)
	if runnable(path) {
		return reveal(path)
	}
	return []string{xdgOpen(), path}
}

// PDFArgv is the command that shows a PDF (GUI-035). Only a plain file named .pdf, which
// nothing would run, is opened; any other is revealed in its folder.
func PDFArgv(path string) []string {
	path = absolute(path)
	if runnable(path) || strings.ToLower(filepath.Ext(path)) != ".pdf" {
		return reveal(path)
	}
	return []string{xdgOpen(), path}
}

// FolderArgv is the command that shows a folder in the file manager. Only a directory
// is opened, and not one named like something that runs; any other path is revealed
// in its parent. A project is someone else's code, and this is one click.
func FolderArgv(path string) []string {
	path = absolute(path)
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".desktop", ".appimage":
		default:
			return []string{xdgOpen(), path}
		}
	}
	return reveal(path)
}
