package windows

import (
	"bytes"
	"strconv"
	"testing"
)

// A value longer than a credential holds is cut into parts that join back whole, and
// the first part's comment says how many there are.
func TestParts(t *testing.T) {
	for _, n := range []int{0, 1, maxBlob, maxBlob + 1, 3*maxBlob + 7} {
		v := bytes.Repeat([]byte("k"), n)
		chunks := split(v)
		for _, c := range chunks {
			if len(c) > maxBlob {
				t.Fatalf("%d bytes: a part of %d bytes", n, len(c))
			}
		}
		if got := join(chunks); !bytes.Equal(got, v) {
			t.Errorf("%d bytes: joined back to %d", n, len(got))
		}
		if got := partCount("Freeloop; parts=" + strconv.Itoa(len(chunks))); got != len(chunks) {
			t.Errorf("%d bytes: partCount = %d, want %d", n, got, len(chunks))
		}
	}
	if partCount("") != 1 || partCount("parts=x") != 1 || partCount("parts=0") != 1 {
		t.Error("a comment without a count must mean one part")
	}
	if !isPart(partName("freeloop/vault", 2)) || isPart("freeloop/vault") {
		t.Error("isPart")
	}
}
