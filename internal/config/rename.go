package config

import (
	"os"
	"time"
)

// replaceFile moves from over to, trying again for a second. On Windows a file
// that another handle has open without sharing deletion cannot be replaced, and
// the settings watcher, a search indexer or a virus scanner may hold one open for
// a moment (NFR-XPLAT-003). Elsewhere the first try is the only one that fails.
func replaceFile(from, to string) error {
	err := os.Rename(from, to)
	for i := 0; err != nil && i < 10 && !os.IsNotExist(err); i++ {
		time.Sleep(100 * time.Millisecond)
		err = os.Rename(from, to)
	}
	return err
}
