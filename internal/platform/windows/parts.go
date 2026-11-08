package windows

import (
	"strconv"
	"strings"
)

// maxBlob is the most a Windows credential holds (CRED_MAX_CREDENTIAL_BLOB_SIZE).
const maxBlob = 5 * 512

// partSuffix ends the name of each part of a long value after the first.
const partSuffix = "#freeloop-part-"

// split cuts a value into parts a credential each holds; an empty value is one part.
func split(b []byte) [][]byte {
	chunks := [][]byte{}
	for len(b) > maxBlob {
		chunks = append(chunks, b[:maxBlob])
		b = b[maxBlob:]
	}
	return append(chunks, b)
}

func join(chunks [][]byte) []byte {
	var out []byte
	for _, c := range chunks {
		out = append(out, c...)
	}
	return out
}

func partName(name string, i int) string { return name + partSuffix + strconv.Itoa(i) }

func isPart(name string) bool { return strings.Contains(name, partSuffix) }

// partCount reads how many parts a first part's comment says there are; 1 when it says
// nothing, as for a credential written by something else.
func partCount(comment string) int {
	_, n, ok := strings.Cut(comment, "parts=")
	if !ok {
		return 1
	}
	c, err := strconv.Atoi(n)
	if err != nil || c < 1 {
		return 1
	}
	return c
}
