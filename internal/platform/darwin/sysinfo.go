//go:build darwin

package darwin

/*
#include <stdlib.h>
#include <stdint.h>
#include <sys/sysctl.h>

// text writes the string sysctl name into buf, at most n bytes, and returns its
// length without the ending zero, or -1.
static int text(const char *name, char *buf, size_t n) {
	size_t size = n;
	if (sysctlbyname(name, buf, &size, NULL, 0) != 0 || size == 0) return -1;
	return buf[size - 1] == 0 ? (int)size - 1 : (int)size;
}

// memsize returns the Mac's memory in bytes, or 0.
static uint64_t memsize(void) {
	uint64_t v = 0;
	size_t size = sizeof(v);
	if (sysctlbyname("hw.memsize", &v, &size, NULL, 0) != 0) return 0;
	return v;
}
*/
import "C"

import (
	"fmt"
	"strings"
	"unsafe"
)

// SystemInfo describes this Mac for a problem report: the macOS version and
// build, the model, the chip and the memory. Nothing that names the user or
// the machine.
func SystemInfo() string {
	parts := []string{}
	if v := sysctlText("kern.osproductversion"); v != "" {
		os := "macOS " + v
		if b := sysctlText("kern.osversion"); b != "" {
			os += " (" + b + ")"
		}
		parts = append(parts, os)
	}
	for _, name := range []string{"hw.model", "machdep.cpu.brand_string"} {
		if v := sysctlText(name); v != "" {
			parts = append(parts, v)
		}
	}
	if gb := memoryGB(); gb > 0 {
		parts = append(parts, fmt.Sprintf("%d GB", gb))
	}
	return strings.Join(parts, ", ")
}

// sysctlText is the string sysctl name, "" if there is none.
func sysctlText(name string) string {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))
	buf := make([]byte, 256)
	n := C.text(cname, (*C.char)(unsafe.Pointer(&buf[0])), C.size_t(len(buf)))
	if n < 0 {
		return ""
	}
	return strings.TrimSpace(string(buf[:n]))
}

// memoryGB is the Mac's memory in whole gigabytes.
func memoryGB() uint64 { return uint64(C.memsize()) >> 30 }
