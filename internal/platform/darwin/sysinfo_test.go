//go:build darwin

package darwin

import (
	"strings"
	"testing"
)

func TestSystemInfoNamesTheSystemAndTheMac(t *testing.T) {
	got := SystemInfo()
	if !strings.HasPrefix(got, "macOS ") || !strings.Contains(got, "Mac") || !strings.HasSuffix(got, " GB") {
		t.Errorf("SystemInfo() = %q", got)
	}
	t.Log(got)
	if gb := memoryGB(); gb < 4 || gb > 4096 {
		t.Errorf("memoryGB() = %d", gb)
	}
}
