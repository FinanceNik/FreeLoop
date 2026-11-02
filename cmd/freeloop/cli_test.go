package main

import "testing"

// What the command line prints comes from the model.
func TestOutputCannotDisguiseItself(t *testing.T) {
	if got := plainText("safe\x1b[2K‮evil​\n"); got != "safe[2Kevil\n" {
		t.Errorf("plainText = %q", got)
	}
	if got := oneLine("Run ls\nperm_1  ses_1  Run rm -rf  [allow_once]"); got != "Run ls perm_1 ses_1 Run rm -rf [allow_once]" {
		t.Errorf("oneLine = %q", got)
	}
}
