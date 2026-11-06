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
*/
import "C"

import "time"

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
