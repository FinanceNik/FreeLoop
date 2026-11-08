package environment

import (
	"runtime"
	"testing"
)

func skipUnlessWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("Windows only")
	}
}
