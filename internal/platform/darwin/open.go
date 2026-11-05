//go:build darwin

package darwin

import "path/filepath"

// EditorArgv is the command that opens a file in the user's default text editor.
// Plain `open` asks Launch Services for the file's own handler, and a .command
// runs in Terminal, behind any name a symbolic link gives it, with no prompt. -t
// only ever edits. The path is absolute for the same reason as sandboxExec.
func EditorArgv(path string) []string {
	return []string{"/usr/bin/open", "-t", path}
}

// PDFArgv is the command that shows a PDF. Named, for the same reason: Preview only shows
// a file, whatever it really is, where Launch Services would go by its handler (GUI-035).
func PDFArgv(path string) []string {
	return []string{"/usr/bin/open", "-a", "Preview", path}
}

// FolderArgv is the command that shows a folder in the Finder. Opening a folder
// named like an app runs that app, -a Finder included, as a test proved; revealing
// it never does. So a name with an extension is revealed in its parent, and any
// other is opened. A project is someone else's code, and this is one click.
func FolderArgv(path string) []string {
	if filepath.Ext(path) != "" {
		return []string{"/usr/bin/open", "-R", path}
	}
	return []string{"/usr/bin/open", "-a", "Finder", path}
}
