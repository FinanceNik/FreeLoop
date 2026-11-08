//go:build linux

package linux

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// SystemInfo describes this machine for a problem report: the distribution and its
// version, the kernel, the processor and the memory. Nothing that names the user or the
// machine.
func SystemInfo() string {
	return describe(read("/etc/os-release"), read("/proc/sys/kernel/osrelease"), read("/proc/cpuinfo"), read("/proc/meminfo"), runtime.GOARCH)
}

func read(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// describe builds the line from the text of the files; arch names the processor when
// cpuinfo does not (the arm64 kernel lists only numbers).
func describe(osRelease, kernel, cpuinfo, meminfo, arch string) string {
	var parts []string
	if name := osReleaseName(osRelease); name != "" {
		parts = append(parts, name)
	}
	if k := strings.TrimSpace(kernel); k != "" {
		parts = append(parts, "Linux "+k)
	}
	if cpu := cpuModel(cpuinfo, arch); cpu != "" {
		parts = append(parts, cpu)
	}
	if gb := memoryGB(meminfo); gb > 0 {
		parts = append(parts, fmt.Sprintf("%d GB", gb))
	}
	return strings.Join(parts, ", ")
}

// osReleaseName is PRETTY_NAME of an os-release file, falling back to NAME and VERSION.
func osReleaseName(text string) string {
	vals := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		vals[k] = v
	}
	if v := vals["PRETTY_NAME"]; v != "" {
		return v
	}
	return strings.TrimSpace(vals["NAME"] + " " + vals["VERSION"])
}

// implementers names the makers of the arm cores the kernel lists by number.
var implementers = map[string]string{
	"0x41": "Arm", "0x43": "Cavium", "0x46": "Fujitsu", "0x48": "HiSilicon", "0x50": "Ampere (APM)",
	"0x51": "Qualcomm", "0x61": "Apple", "0xc0": "Ampere",
}

// cpuModel is the processor's model name. x86 lists it; arm64 does not, so there it is
// the architecture with the core's implementer and part numbers, which say what chip it is.
func cpuModel(cpuinfo, arch string) string {
	var impl, part string
	for _, line := range strings.Split(cpuinfo, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "model name", "Model Name", "cpu model", "Hardware":
			if v != "" {
				return v
			}
		case "CPU implementer":
			if impl == "" {
				impl = v
			}
		case "CPU part":
			if part == "" {
				part = v
			}
		}
	}
	if impl != "" && part != "" {
		if name, ok := implementers[strings.ToLower(impl)]; ok {
			impl = name
		}
		return fmt.Sprintf("%s (%s, part %s)", arch, impl, part)
	}
	return arch
}

// memoryGB is the memory in MemTotal, in whole gigabytes, rounded to the nearest: the
// kernel reports a little less than is installed.
func memoryGB(meminfo string) uint64 {
	for _, line := range strings.Split(meminfo, "\n") {
		if rest, ok := strings.CutPrefix(line, "MemTotal:"); ok {
			f := strings.Fields(rest)
			if len(f) == 0 {
				return 0
			}
			kb, err := strconv.ParseUint(f[0], 10, 64)
			if err != nil {
				return 0
			}
			return (kb + 512*1024) >> 20
		}
	}
	return 0
}
