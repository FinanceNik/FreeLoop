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

// A file does not choose what opens it: a script named .pdf must not run.
func TestPDFArgvNamesTheApp(t *testing.T) {
	got := PDFArgv("/p/run.command.pdf")
	want := []string{"/usr/bin/open", "-a", "Preview", "/p/run.command.pdf"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v", got)
		}
	}
}
