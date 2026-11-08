package main

import (
	"os"
	"slices"
	"strings"
)

// shipped is "1" in the build users get: `make dmg` and `make release`, the CI
// release job and verify-repro with it, link it in with -X main.shipped=1
// (build/darwin/Taskfile.yml). A shipped build reads none of the development
// switches. `make build`, `make package`, `make dev` and the served build of
// `make check-app` leave it empty.
var shipped string

// devEnv is the one way the app reads a development switch: FREELOOP_DEV and
// FREELOOP_DEV_*, FREELOOP_RELEASE_API and FREELOOP_HOME. It is "" in a shipped
// build, whatever the environment says, so there the demo provider, the stand-in
// update server and GitHub CLI and a data folder other than the standard one
// cannot be turned on from outside. internal/archtest keeps every other read of a
// FREELOOP_ variable out of this package.
func devEnv(name string) string {
	return devValue(shipped == "1", name, os.Getenv)
}

func devValue(isShipped bool, name string, getenv func(string) string) string {
	if isShipped {
		return ""
	}
	return getenv(name)
}

// isDevSwitch says whether an environment variable is one devEnv reads.
func isDevSwitch(name string) bool {
	return name == "FREELOOP_DEV" || strings.HasPrefix(name, "FREELOOP_DEV_") ||
		name == "FREELOOP_RELEASE_API" || name == "FREELOOP_HOME"
}

// ignoredDevSwitches names, sorted, the development switches set in environ
// ("NAME=value" entries) that a shipped build ignores; none in any other build.
// Only names: a value can be a path or an address.
func ignoredDevSwitches(isShipped bool, environ []string) []string {
	if !isShipped {
		return nil
	}
	var out []string
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if isDevSwitch(name) && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}
