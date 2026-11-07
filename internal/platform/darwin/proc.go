//go:build darwin

package darwin

/*
#include <libproc.h>
#include <sys/proc_info.h>

// start returns when process pid started, in microseconds since 1970, or 0.
static long long start(int pid) {
	struct proc_bsdinfo info;
	if (proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, &info, PROC_PIDTBSDINFO_SIZE) != PROC_PIDTBSDINFO_SIZE) return 0;
	if ((int)info.pbi_pid != pid) return 0;
	return (long long)info.pbi_start_tvsec * 1000000 + (long long)info.pbi_start_tvusec;
}

// members writes the processes in group pgid to pids, at most n, and returns how many.
static int members(int pgid, int *pids, int n) {
	int bytes = proc_listpids(PROC_PGRP_ONLY, (uint32_t)pgid, pids, n * (int)sizeof(int));
	return bytes <= 0 ? 0 : bytes / (int)sizeof(int);
}
*/
import "C"

import (
	"time"
	"unsafe"
)

// ProcessStart returns when process pid started, which tells it from a later process
// that was given the same number (NFR-REL-005). false: no such process, or one this
// user may not look at.
func ProcessStart(pid int) (time.Time, bool) {
	if pid <= 0 {
		return time.Time{}, false
	}
	us := int64(C.start(C.int(pid)))
	if us == 0 {
		return time.Time{}, false
	}
	return time.UnixMicro(us), true
}

// GroupMembers lists the processes in process group pgid (NFR-REL-005).
func GroupMembers(pgid int) []int {
	if pgid <= 1 {
		return nil
	}
	buf := make([]C.int, 1024)
	n := int(C.members(C.int(pgid), (*C.int)(unsafe.Pointer(&buf[0])), C.int(len(buf))))
	out := make([]int, 0, n)
	for _, p := range buf[:n] {
		if p > 0 {
			out = append(out, int(p))
		}
	}
	return out
}
