package environment

import "testing"

// Each pair opens one file on APFS.
func TestFoldPathFollowsAPFS(t *testing.T) {
	for _, pair := range [][2]string{
		{"passwords.txt", "paßwords.txt"}, {"PASSWORDS", "paẞwords"}, {".ssh", ".ßh"}, {".config", ".conﬁg"}, {"office", "oﬃce"}, {"aster", "aﬅer"},
		{".ssh", ".ſsh"}, {"key", "Key"}, {"café", "café"}, {".GIT", ".git"},
	} {
		if FoldPath(pair[0]) != FoldPath(pair[1]) {
			t.Errorf("%q and %q differ: %q, %q", pair[0], pair[1], FoldPath(pair[0]), FoldPath(pair[1]))
		}
	}
	if FoldPath("a.txt") == FoldPath("b.txt") {
		t.Error("everything folds to one name")
	}
}
