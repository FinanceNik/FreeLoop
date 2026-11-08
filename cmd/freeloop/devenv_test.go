package main

import (
	"slices"
	"testing"
)

func TestDevValueIgnoresTheEnvironmentWhenShipped(t *testing.T) {
	env := map[string]string{
		"FREELOOP_DEV": "1", "FREELOOP_DEV_GH": "/tmp/gh", "FREELOOP_DEV_SECRETS": "memory",
		"FREELOOP_DEV_BACKUP": "/tmp/b.json", "FREELOOP_RELEASE_API": "http://127.0.0.1:1", "FREELOOP_HOME": "/tmp/home",
	}
	getenv := func(name string) string { return env[name] }
	for name, value := range env {
		if got := devValue(false, name, getenv); got != value {
			t.Errorf("development build: %s = %q, want %q", name, got, value)
		}
		if got := devValue(true, name, getenv); got != "" {
			t.Errorf("shipped build: %s = %q, want it ignored", name, got)
		}
		if !isDevSwitch(name) {
			t.Errorf("%s is not counted as a development switch", name)
		}
	}
}

func TestIgnoredDevSwitchesNamesOnlySwitches(t *testing.T) {
	environ := []string{"HOME=/Users/x", "FREELOOP_HOME=/tmp/secret-path", "FREELOOP_DEV=1", "FREELOOP_DEVELOPER=1",
		"FREELOOP_DEV_GH=/bin/gh", "FREELOOP_RELEASE_API=http://x", "FREELOOP_PORT=3000", "FREELOOP_DEV=1"}
	want := []string{"FREELOOP_DEV", "FREELOOP_DEV_GH", "FREELOOP_HOME", "FREELOOP_RELEASE_API"}
	if got := ignoredDevSwitches(true, environ); !slices.Equal(got, want) {
		t.Errorf("shipped: %v, want %v", got, want)
	}
	if got := ignoredDevSwitches(false, environ); got != nil {
		t.Errorf("development build ignores %v", got)
	}
}

// devEnv follows the link-time flag: a plain `go test` reads the switches, and
// one linked as the shipped app is ignores them. A test binary names this package
// by its import path, not main:
// go test -ldflags=-X=github.com/FinanceNik/Freeloop/cmd/freeloop.shipped=1 ./cmd/freeloop
func TestDevEnvFollowsTheLinkTimeFlag(t *testing.T) {
	t.Setenv("FREELOOP_DEV_GH", "/tmp/gh")
	want := "/tmp/gh"
	if shipped == "1" {
		want = ""
	}
	t.Logf("shipped = %q", shipped)
	if got := devEnv("FREELOOP_DEV_GH"); got != want {
		t.Errorf("shipped = %q: devEnv = %q, want %q", shipped, got, want)
	}
}
