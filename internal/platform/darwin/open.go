//go:build darwin

package darwin

// EditorArgv is the command that opens a file in the user's default text editor.
// Plain `open` asks Launch Services for the file's own handler, and a .command
// runs in Terminal, behind any name a symbolic link gives it, with no prompt. -t
// only ever edits. The path is absolute for the same reason as sandboxExec.
func EditorArgv(path string) []string {
	return []string{"/usr/bin/open", "-t", path}
}
