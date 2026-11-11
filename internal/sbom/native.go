package sbom

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// Native is a program the app carries that is neither Go nor npm: on a Mac,
// whisper.cpp's whisper-cli, built from its pinned source for voice input (GUI-050).
// scripts/whisper-cli.sh describes it beside the program, in whisper-cli.json, with
// its license file.
type Native struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	PURL    string `json:"purl"`
	// SHA256 is the source archive's.
	SHA256  string `json:"sha256"`
	License string `json:"license"`
	// Includes names what its source carries of others', and their licenses.
	Includes string `json:"includes"`
	// LicenseFile is the license's text, beside the description.
	LicenseFile string `json:"licenseFile"`
}

// ReadNative reads a native program's description from file in root.
func ReadNative(root fs.FS, file string) (Native, error) {
	b, err := fs.ReadFile(root, file)
	if err != nil {
		return Native{}, err
	}
	var n Native
	if err := json.Unmarshal(b, &n); err != nil {
		return Native{}, fmt.Errorf("%s: %w", file, err)
	}
	if n.Name == "" || n.Version == "" || len(n.SHA256) != 64 || n.License == "" || n.LicenseFile == "" || strings.Contains(n.LicenseFile, "/") {
		return Native{}, fmt.Errorf("%s does not describe a program fully", file)
	}
	n.LicenseFile = path.Join(path.Dir(file), n.LicenseFile)
	return n, nil
}

func (n Native) component() component {
	c := component{Type: "application", BOMRef: n.PURL, Name: n.Name, Version: n.Version, PURL: n.PURL, Hashes: []hash{{Alg: "SHA-256", Content: n.SHA256}}}
	c.Properties = []property{{Name: "license", Value: n.License}}
	if n.Includes != "" {
		c.Properties = append(c.Properties, property{Name: "includes", Value: n.Includes})
	}
	return c
}

// noticeOf is a native program's license, for the notices.
func (n Native) noticeOf(root fs.FS) (string, error) {
	if root == nil {
		return "", errors.New("no folder to read " + n.Name + "'s license from")
	}
	b, err := fs.ReadFile(root, n.LicenseFile)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(b))
	if n.Includes != "" {
		text += "\n\nIt includes " + n.Includes + "."
	}
	return text, nil
}
