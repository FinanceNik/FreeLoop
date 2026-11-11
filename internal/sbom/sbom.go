// Package sbom writes the software bill of materials of a build (SEC-012): what
// is really in the app, in CycloneDX JSON. Go modules come from the binary's own
// build information, so a module the linker left out is not listed and one it
// took is, and the frontend's from its lockfile, without the packages only its
// build uses. Nothing in it depends on when it was made, so a reproducible
// build gives the same one (NFR-SEC-004).
package sbom

import (
	"cmp"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"runtime/debug"
	"slices"
	"strings"
)

// App names the application the bill is for.
type App struct {
	Name, Version string
}

type hash struct {
	Alg     string `json:"alg"`
	Content string `json:"content"`
}

type property struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type component struct {
	Type       string     `json:"type"`
	BOMRef     string     `json:"bom-ref,omitempty"`
	Name       string     `json:"name"`
	Version    string     `json:"version,omitempty"`
	PURL       string     `json:"purl,omitempty"`
	Hashes     []hash     `json:"hashes,omitempty"`
	Properties []property `json:"properties,omitempty"`
}

type bom struct {
	BOMFormat   string      `json:"bomFormat"`
	SpecVersion string      `json:"specVersion"`
	Version     int         `json:"version"`
	Metadata    metadata    `json:"metadata"`
	Components  []component `json:"components"`
}

type metadata struct {
	Component component `json:"component"`
}

type lockFile struct {
	Packages map[string]struct {
		Version   string `json:"version"`
		Integrity string `json:"integrity"`
		Dev       bool   `json:"dev"`
		DevOpt    bool   `json:"devOptional"`
		Link      bool   `json:"link"`
	} `json:"packages"`
}

// Build returns the bill for a binary's build information, the frontend's
// package-lock.json and the native programs the app carries.
func Build(app App, info *debug.BuildInfo, packageLock []byte, natives ...Native) ([]byte, error) {
	if info == nil {
		return nil, errors.New("no build information")
	}
	var out []component
	goVersion := strings.TrimPrefix(info.GoVersion, "go")
	out = append(out, component{Type: "library", BOMRef: "pkg:golang/stdlib@" + goVersion, Name: "stdlib", Version: goVersion, PURL: "pkg:golang/stdlib@" + goVersion})
	for _, d := range info.Deps {
		// A replace directive means the code came from the replacement.
		if d.Replace != nil {
			d = d.Replace
		}
		purl := "pkg:golang/" + purlPath(d.Path) + "@" + d.Version
		c := component{Type: "library", BOMRef: purl, Name: d.Path, Version: d.Version, PURL: purl}
		if d.Sum != "" {
			// go.sum's h1: hash covers a module's files, not an archive, so it
			// fits none of CycloneDX's hash algorithms.
			c.Properties = []property{{Name: "go:module:h1", Value: d.Sum}}
		}
		out = append(out, c)
	}
	var lock lockFile
	if err := json.Unmarshal(packageLock, &lock); err != nil {
		return nil, fmt.Errorf("package-lock.json: %w", err)
	}
	for key, p := range lock.Packages {
		name, ok := strings.CutPrefix(key, "node_modules/")
		if !ok || p.Dev || p.DevOpt || p.Link {
			continue
		}
		// A nested copy is node_modules/a/node_modules/b.
		if i := strings.LastIndex(name, "node_modules/"); i >= 0 {
			name = name[i+len("node_modules/"):]
		}
		purl := "pkg:npm/" + purlPath(name) + "@" + p.Version
		c := component{Type: "library", BOMRef: purl, Name: name, Version: p.Version, PURL: purl}
		h, err := integrity(p.Integrity)
		if err != nil {
			return nil, fmt.Errorf("package-lock.json: %s: %w", key, err)
		}
		c.Hashes = []hash{h}
		out = append(out, c)
	}
	for _, n := range natives {
		out = append(out, n.component())
	}
	slices.SortFunc(out, func(a, b component) int {
		return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.Version, b.Version))
	})
	out = slices.CompactFunc(out, func(a, b component) bool { return a.BOMRef == b.BOMRef })
	b, err := json.MarshalIndent(bom{
		BOMFormat: "CycloneDX", SpecVersion: "1.6", Version: 1,
		Metadata:   metadata{Component: component{Type: "application", Name: app.Name, Version: app.Version}},
		Components: out,
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// purlPath percent-encodes a path's segments as a package URL wants, keeping
// the slashes between them.
func purlPath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = strings.ReplaceAll(url.PathEscape(s), "@", "%40")
	}
	return strings.Join(parts, "/")
}

// integrity turns a lockfile's "sha512-<base64>" into a CycloneDX hash, which
// is hex.
func integrity(s string) (hash, error) {
	alg, value, ok := strings.Cut(s, "-")
	if !ok {
		return hash{}, fmt.Errorf("no usable integrity hash: %q", s)
	}
	names := map[string]string{"sha512": "SHA-512", "sha384": "SHA-384", "sha256": "SHA-256"}
	name, ok := names[alg]
	if !ok {
		return hash{}, fmt.Errorf("integrity hash %s is too weak to list", alg)
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return hash{}, fmt.Errorf("integrity hash: %w", err)
	}
	return hash{Alg: name, Content: hex.EncodeToString(raw)}, nil
}
