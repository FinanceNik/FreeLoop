//go:build unix

package environment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// groupAlive reports whether any process of group pgid still runs.
func groupAlive(pgid int) bool {
	return pgid > 1 && syscall.Kill(-pgid, 0) == nil
}

// stopGroup asks group pgid to end and kills what is left a moment later.
func stopGroup(pgid int) {
	if !groupAlive(pgid) {
		return
	}
	syscall.Kill(-pgid, syscall.SIGTERM)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && groupAlive(pgid) {
		time.Sleep(50 * time.Millisecond)
	}
	syscall.Kill(-pgid, syscall.SIGKILL)
}

// ReapLeftovers stops the process groups path says were running when the app last
// ended without stopping them, and returns what it stopped. Each is asked to end,
// then killed if still there a moment later.
func ReapLeftovers(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []ledgerEntry
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("environment: %s: %w", filepath.Base(path), err)
	}
	var stopped []int
	var names []string
	for _, e := range list {
		if e.Pgid <= 1 || ProcessStarted == nil || !leftover(e) {
			continue
		}
		if syscall.Kill(-e.Pgid, syscall.SIGTERM) == nil {
			stopped = append(stopped, e.Pgid)
			names = append(names, e.Command)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for _, pgid := range stopped {
		for time.Now().Before(deadline) && syscall.Kill(-pgid, 0) == nil {
			time.Sleep(50 * time.Millisecond)
		}
		syscall.Kill(-pgid, syscall.SIGKILL)
	}
	return names, nil
}
