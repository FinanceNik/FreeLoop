//go:build darwin

package darwin

import (
	"slices"
	"testing"
)

// Opening a folder named like an app runs it, so such a name is only revealed.
func TestFolderArgvNeverOpensABundle(t *testing.T) {
	for path, flag := range map[string]string{
		"/x/Foo.app":     "-R",
		"/x/Foo.command": "-R",
		"/x/my.project":  "-R",
		"/x/freeloop":    "-a",
	} {
		argv := FolderArgv(path)
		if argv[0] != "/usr/bin/open" || !slices.Contains(argv, flag) || argv[len(argv)-1] != path {
			t.Errorf("%s: %v", path, argv)
		}
	}
}
