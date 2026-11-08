package windows

import (
	"os"
	"path/filepath"
	"strings"
)

// Windows opens a file with the program its type is registered to, and many types run
// what they open: a program, a script, a shortcut, an installer, and on many machines a
// .py or .sh file too (Python's launcher, Git Bash). So nothing is ever opened by its
// type here except a PDF: a file to edit goes to Notepad, the text editor every Windows
// has, as a Mac's goes to its text editor, and a folder is shown in File Explorer.
// ShellExec_RunDLL opens a folder or PDF as a double-click would, and, unlike
// explorer.exe, which exits with 1 when all went well, says whether it did.

// systemRoot is the Windows folder, by its variable as the system sets it.
func systemRoot() string {
	if r := os.Getenv("SystemRoot"); r != "" && filepath.IsAbs(r) {
		return r
	}
	return `C:\Windows`
}

func system32(program string) string { return filepath.Join(systemRoot(), "System32", program) }

// shellOpen is the command that opens path as a double-click in File Explorer would.
func shellOpen(path string) []string {
	return []string{system32("rundll32.exe"), "shell32.dll,ShellExec_RunDLL", path}
}

// absolute makes path absolute, so a name that begins with a dash or a slash is never
// an option.
func absolute(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// isDir reports a folder, following links. A folder named like a program is still a
// folder: opening it shows it.
func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// isPlainFile reports a regular file, following links.
func isPlainFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

// EditorArgv is the command that opens a file in Notepad, which only ever shows it.
// What is not a plain file has its folder shown instead.
func EditorArgv(path string) []string {
	path = absolute(path)
	if !isPlainFile(path) {
		return FolderArgv(filepath.Dir(path))
	}
	return []string{system32("notepad.exe"), path}
}

// PDFArgv is the command that shows a PDF (GUI-035) in the app registered for PDFs.
// Only a plain file named .pdf, and not a link to something else, is opened; any other
// has its folder shown.
func PDFArgv(path string) []string {
	path = absolute(path)
	real := path
	if r, err := filepath.EvalSymlinks(path); err == nil {
		real = r
	}
	if !isPlainFile(path) || strings.ToLower(filepath.Ext(path)) != ".pdf" || strings.ToLower(filepath.Ext(real)) != ".pdf" {
		return FolderArgv(filepath.Dir(path))
	}
	return shellOpen(path)
}

// FolderArgv is the command that shows a folder in File Explorer; any other path has
// the folder that holds it shown.
func FolderArgv(path string) []string {
	path = absolute(path)
	for !isDir(path) {
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	return shellOpen(path)
}
