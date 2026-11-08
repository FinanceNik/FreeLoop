//go:build linux

// Package linux holds the Linux-only code (NFR-XPLAT-002, NFR-XPLAT-003): the process
// table, opening files, the system description and the Secret Service.
package linux

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// clockTick is the unit of the start time in /proc/<pid>/stat. That is USER_HZ, which
// the kernel fixes at 100 for /proc on every architecture Linux runs on, whatever the
// kernel's own timer frequency is; it is not what sysconf(_SC_CLK_TCK) could differ
// from, so there is no need for cgo to ask.
const clockTick = 100

var (
	bootOnce sync.Once
	bootTime time.Time
)

// boot is when the system started, read once. The kernel works btime out from the wall
// clock, so a clock that is stepped moves it by a second or so; reading it once keeps a
// process's start the same on every call within a run. Across runs, a start that moved
// is not equal to the one recorded, and the ledger then leaves that group alone, which
// errs toward stopping too little.
func boot() time.Time {
	bootOnce.Do(func() {
		b, err := os.ReadFile("/proc/stat")
		if err != nil {
			return
		}
		for _, line := range bytes.Split(b, []byte("\n")) {
			if v, ok := bytes.CutPrefix(line, []byte("btime ")); ok {
				if sec, err := strconv.ParseInt(string(bytes.TrimSpace(v)), 10, 64); err == nil && sec > 0 {
					bootTime = time.Unix(sec, 0)
				}
				return
			}
		}
	})
	return bootTime
}

// statFields returns the fields of /proc/<pid>/stat from the third on (the state), as
// the file's text split by spaces. The second field, the command's name, can hold
// spaces and parentheses, so the fields are counted from the last ')'.
func statFields(pid int) []string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return nil
	}
	return parseStat(string(b))
}

func parseStat(s string) []string {
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return nil
	}
	return strings.Fields(s[i+1:])
}

// Fields of /proc/<pid>/stat after the command's name, numbered from the state (field 3).
const (
	fieldPgrp      = 2  // field 5
	fieldStartTime = 19 // field 22
)

// ProcessStart returns when process pid started, which tells it from a later process
// that was given the same number (NFR-REL-005). false: no such process. The time is
// the boot time plus the process's start in clock ticks since boot, and the same on
// every call.
func ProcessStart(pid int) (time.Time, bool) {
	if pid <= 0 {
		return time.Time{}, false
	}
	f := statFields(pid)
	if len(f) <= fieldStartTime {
		return time.Time{}, false
	}
	ticks, err := strconv.ParseInt(f[fieldStartTime], 10, 64)
	b := boot()
	if err != nil || b.IsZero() {
		return time.Time{}, false
	}
	return b.Add(time.Duration(ticks) * (time.Second / clockTick)), true
}

// GroupMembers lists the processes in process group pgid (NFR-REL-005), by looking
// at the process group field of every process.
func GroupMembers(pgid int) []int {
	if pgid <= 1 {
		return nil
	}
	dir, err := os.Open("/proc")
	if err != nil {
		return nil
	}
	defer dir.Close()
	names, err := dir.Readdirnames(-1)
	if err != nil {
		return nil
	}
	var out []int
	for _, name := range names {
		pid, err := strconv.Atoi(name)
		if err != nil || pid <= 0 {
			continue
		}
		f := statFields(pid)
		if len(f) <= fieldPgrp {
			continue
		}
		// A zombie still holds its group, but nothing in it runs.
		if f[0] == "Z" || f[0] == "X" {
			continue
		}
		if g, err := strconv.Atoi(f[fieldPgrp]); err == nil && g == pgid {
			out = append(out, pid)
		}
	}
	return out
}
