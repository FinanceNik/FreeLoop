package environment

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// On Windows a command's group is a job object (NFR-REL-005): the command starts
// suspended, joins a job of its own and then runs, so everything it starts is in the
// job too. Each job ends its processes when its last handle closes, and only the app
// holds one, so a crash or a forced quit of the app ends them all: what the ledger of
// process groups does on Unix, the system does here, and nothing is written down.

var jobs struct {
	mu sync.Mutex
	// byPid holds each group's job, by the process that leads it.
	byPid map[int]windows.Handle
}

// setProcessGroup starts the command suspended, with no console window of its own:
// the app has none, and a console program would otherwise open a window for each
// command.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED | windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP
}

// joinGroup puts the suspended command in a new job and lets it run.
func joinGroup(cmd *exec.Cmd) error {
	pid := cmd.Process.Pid
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)
	if err := addToNewJob(pid, h); err != nil {
		return err
	}
	if r, _, _ := procNtResumeProcess.Call(uintptr(h)); r != 0 {
		return fmt.Errorf("resume process %d: NTSTATUS 0x%x", pid, r)
	}
	return nil
}

// addToNewJob makes a job that ends its processes when its last handle closes, puts
// process h in it and records it as the group pid leads.
func addToNewJob(pid int, h windows.Handle) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create a job: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return fmt.Errorf("set up a job: %w", err)
	}
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		windows.CloseHandle(job)
		return fmt.Errorf("put process %d in a job: %w", pid, err)
	}
	jobs.mu.Lock()
	if jobs.byPid == nil {
		jobs.byPid = map[int]windows.Handle{}
	}
	jobs.byPid[pid] = job
	jobs.mu.Unlock()
	return nil
}

// NtResumeProcess resumes every thread of a process; os/exec keeps the handle of the
// main thread of a process it started suspended to itself.
var procNtResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

func jobOf(pid int) (windows.Handle, bool) {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	job, ok := jobs.byPid[pid]
	return job, ok
}

// killProcessGroup ends the command and everything it started.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	if job, ok := jobOf(cmd.Process.Pid); ok {
		windows.TerminateJobObject(job, 1)
		return
	}
	cmd.Process.Kill()
}

// groupAlive reports whether any process of the group led by pid still runs.
func groupAlive(pid int) bool {
	job, ok := jobOf(pid)
	if !ok {
		return false
	}
	var info jobAccounting
	if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil {
		return false
	}
	return info.ActiveProcesses > 0
}

// jobAccounting is JOBOBJECT_BASIC_ACCOUNTING_INFORMATION, which x/sys/windows does
// not define.
type jobAccounting struct {
	TotalUserTime, TotalKernelTime, ThisPeriodTotalUserTime, ThisPeriodTotalKernelTime int64
	TotalPageFaultCount, TotalProcesses, ActiveProcesses, TotalTerminatedProcesses     uint32
}

// stopGroup ends the group led by pid. A console program run without a console
// cannot be asked politely, so it is ended at once.
func stopGroup(pid int) {
	jobs.mu.Lock()
	job, ok := jobs.byPid[pid]
	delete(jobs.byPid, pid)
	jobs.mu.Unlock()
	if ok {
		windows.TerminateJobObject(job, 1)
		windows.CloseHandle(job)
	}
}

// releaseGroup closes the job of a group that has ended. One still running keeps its
// job, so the app's end still ends it.
func releaseGroup(pid int) {
	if groupAlive(pid) {
		return
	}
	jobs.mu.Lock()
	job, ok := jobs.byPid[pid]
	delete(jobs.byPid, pid)
	jobs.mu.Unlock()
	if ok {
		windows.CloseHandle(job)
	}
}

// ReapLeftovers has nothing to stop on Windows: the jobs ended with the app that held
// them.
func ReapLeftovers(string) ([]string, error) { return nil, nil }

// ErrLocked is a data folder another copy of the app has open.
var ErrLocked = errors.New("another copy of Freeloop is using this data folder")

// LockFolder holds a lock on a folder until the process ends, however it ends:
// the system lets go of it. Two copies of the app on one data folder would write
// over each other's transcripts.
func LockFolder(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("environment: %w", err)
	}
	var ol windows.Overlapped
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ol); err != nil {
		f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("environment: %w", err)
	}
	return f, nil
}

// RedirectStderr sends the process's standard error to f, so a crash's stack trace
// lands in the log: a program built for the Windows GUI subsystem has none.
func RedirectStderr(f *os.File) error {
	if err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd())); err != nil {
		return err
	}
	os.Stderr = f
	return nil
}
