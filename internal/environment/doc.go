// Package environment is where a session's processes run and its files live
// (ENV-001). It is the only engine package allowed to start processes, and one
// of the few allowed to touch the file system; internal/archtest enforces that.
// The Environment interface and the local implementation arrive in M1.
package environment
