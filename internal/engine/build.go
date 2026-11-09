package engine

import (
	"runtime/debug"
	"strings"
)

// BuildCommit is the commit this build was made from, as the build tasks link it in
// (-X github.com/FinanceNik/Freeloop/internal/engine.BuildCommit=a59ce34), with
// "+dirty" after it when the files had changes that were not committed. Two machines
// that run builds of the same version can then tell whether they run the same code:
// Settings, the problem report and the app bundle all say it. Empty for a build that
// was not given one, which then reads Go's own record of the repository, if it has
// one.
var BuildCommit string

// dirtySuffix follows a commit made from files with uncommitted changes.
const dirtySuffix = "+dirty"

// BuildInfo says what build is running.
type BuildInfo struct {
	// Version is the release this build is of, which every build of it shares.
	Version string `json:"version"`
	// Commit is the short commit it was built from, empty when unknown.
	Commit string `json:"commit"`
	// Dirty says the files had uncommitted changes, so the commit is not all of it.
	Dirty bool `json:"dirty"`
	// Stamp is the commit as the build tasks write it, with +dirty when it applies.
	Stamp string `json:"stamp"`
	// Label is both, as a person reads them: "0.1.0-beta.1 · a59ce34".
	Label string `json:"label"`
}

// Build reports the running build.
func Build() BuildInfo {
	var settings []debug.BuildSetting
	if info, ok := debug.ReadBuildInfo(); ok {
		settings = info.Settings
	}
	return buildInfo(AppVersion, BuildCommit, settings)
}

// buildInfo is Build for a version, a linked-in commit and Go's VCS settings.
func buildInfo(version, linked string, settings []debug.BuildSetting) BuildInfo {
	commit := strings.TrimSpace(linked)
	if commit == "" {
		commit = vcsCommit(settings)
	}
	commit, dirty := strings.CutSuffix(commit, dirtySuffix)
	b := BuildInfo{Version: version, Commit: commit, Dirty: dirty && commit != "", Label: version}
	if commit != "" {
		b.Stamp = commit
		if b.Dirty {
			b.Stamp += dirtySuffix
		}
		b.Label += " · " + b.Stamp
	}
	return b
}

// vcsCommit reads the commit Go records in a binary built inside a repository.
func vcsCommit(settings []debug.BuildSetting) string {
	var revision, modified string
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if len(revision) < 7 {
		return ""
	}
	if modified == "true" {
		return revision[:7] + dirtySuffix
	}
	return revision[:7]
}
