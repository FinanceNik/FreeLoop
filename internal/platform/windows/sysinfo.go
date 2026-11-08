//go:build windows

package windows

import (
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// SystemInfo describes this machine for a problem report: the Windows edition and
// build, the processor and the memory. Nothing that names the user or the machine.
func SystemInfo() string {
	var parts []string
	v := windows.RtlGetVersion()
	name := "Windows"
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE); err == nil {
		if p, _, err := k.GetStringValue("ProductName"); err == nil && p != "" {
			name = p
		}
		if d, _, err := k.GetStringValue("DisplayVersion"); err == nil && d != "" {
			name += " " + d
		}
		k.Close()
	}
	// Windows 11 still calls itself Windows 10 in ProductName; its builds start at 22000.
	if v.BuildNumber >= 22000 {
		name = strings.Replace(name, "Windows 10", "Windows 11", 1)
	}
	parts = append(parts, fmt.Sprintf("%s (build %d.%d.%d)", name, v.MajorVersion, v.MinorVersion, v.BuildNumber))
	cpu := runtime.GOARCH
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `HARDWARE\DESCRIPTION\System\CentralProcessor\0`, registry.QUERY_VALUE); err == nil {
		if p, _, err := k.GetStringValue("ProcessorNameString"); err == nil && strings.TrimSpace(p) != "" {
			cpu = strings.TrimSpace(p)
		}
		k.Close()
	}
	parts = append(parts, cpu)
	if gb := memoryGB(); gb > 0 {
		parts = append(parts, fmt.Sprintf("%d GB", gb))
	}
	return strings.Join(parts, ", ")
}

// memoryStatus is MEMORYSTATUSEX.
type memoryStatus struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var procGlobalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// memoryGB is the installed memory in whole gigabytes, rounded to the nearest.
func memoryGB() uint64 {
	m := memoryStatus{}
	m.Length = uint32(unsafe.Sizeof(m))
	if r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m))); r == 0 {
		return 0
	}
	return (m.TotalPhys + 512<<20) >> 30
}
