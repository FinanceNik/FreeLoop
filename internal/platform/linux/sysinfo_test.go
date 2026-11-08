//go:build linux

package linux

import (
	"strings"
	"testing"
)

func TestSystemInfoNamesTheSystem(t *testing.T) {
	got := SystemInfo()
	if !strings.Contains(got, "Linux ") || !strings.HasSuffix(got, " GB") {
		t.Errorf("SystemInfo() = %q", got)
	}
	t.Log(got)
}

func TestDescribe(t *testing.T) {
	osr := "NAME=\"Ubuntu\"\nVERSION=\"22.04.5 LTS (Jammy Jellyfish)\"\nPRETTY_NAME=\"Ubuntu 22.04.5 LTS\"\nID=ubuntu\n"
	x86 := "processor\t: 0\nmodel name\t: AMD Ryzen 7 5800X 8-Core Processor\n"
	arm := "processor\t: 0\nBogoMIPS\t: 48.00\nCPU implementer\t: 0x61\nCPU part\t: 0x000\n"
	mem := "MemTotal:       16318620 kB\nMemFree:  100 kB\n"
	for _, c := range []struct{ cpu, arch, want string }{
		{x86, "amd64", "Ubuntu 22.04.5 LTS, Linux 6.8.0-45-generic, AMD Ryzen 7 5800X 8-Core Processor, 16 GB"},
		{arm, "arm64", "Ubuntu 22.04.5 LTS, Linux 6.8.0-45-generic, arm64 (Apple, part 0x000), 16 GB"},
		{"", "riscv64", "Ubuntu 22.04.5 LTS, Linux 6.8.0-45-generic, riscv64, 16 GB"},
	} {
		if got := describe(osr, "6.8.0-45-generic\n", c.cpu, mem, c.arch); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
	if got := osReleaseName("NAME=Alpine\nVERSION=3.20\n"); got != "Alpine 3.20" {
		t.Errorf("no PRETTY_NAME: %q", got)
	}
	if got := describe("", "", "", "", "amd64"); got != "amd64" {
		t.Errorf("nothing readable: %q", got)
	}
}
