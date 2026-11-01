package sbom

import (
	"bytes"
	"encoding/json"
	"runtime/debug"
	"strings"
	"testing"
)

const lock = `{"packages": {
  "": {"name": "frontend"},
  "node_modules/react": {"version": "19.3.1", "integrity": "sha512-3q2+7w=="},
  "node_modules/@wailsio/runtime": {"version": "3.0.0", "integrity": "sha512-3q2+7w=="},
  "node_modules/a/node_modules/react": {"version": "18.0.0", "integrity": "sha512-3q2+7w=="},
  "node_modules/vite": {"version": "8.3.5", "dev": true, "integrity": "sha512-3q2+7w=="}
}}`

func info() *debug.BuildInfo {
	return &debug.BuildInfo{GoVersion: "go1.27.3", Deps: []*debug.Module{
		{Path: "github.com/wailsapp/wails/v3", Version: "v3.0.0", Sum: "h1:abc="},
		{Path: "example.com/old", Version: "v1.0.0", Replace: &debug.Module{Path: "example.com/new", Version: "v1.1.0", Sum: "h1:def="}},
	}}
}

// SEC-012: what the app holds, and nothing only its build used.
func TestBill(t *testing.T) {
	b, err := Build(App{Name: "Freeloop", Version: "0.0.1"}, info(), []byte(lock))
	if err != nil {
		t.Fatal(err)
	}
	var got bom
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.BOMFormat != "CycloneDX" || got.SpecVersion != "1.6" || got.Metadata.Component.Name != "Freeloop" {
		t.Errorf("header = %+v", got)
	}
	refs := map[string]component{}
	for _, c := range got.Components {
		if c.Type != "library" || c.Name == "" {
			t.Errorf("component = %+v", c)
		}
		refs[c.PURL] = c
	}
	for _, want := range []string{
		"pkg:golang/stdlib@1.27.3",
		"pkg:golang/github.com/wailsapp/wails/v3@v3.0.0",
		"pkg:golang/example.com/new@v1.1.0",
		"pkg:npm/react@19.3.1",
		"pkg:npm/react@18.0.0",
		"pkg:npm/%40wailsio/runtime@3.0.0",
	} {
		if _, ok := refs[want]; !ok {
			t.Errorf("%s is missing from %v", want, refs)
		}
	}
	if strings.Contains(string(b), "vite") || strings.Contains(string(b), "example.com/old") {
		t.Errorf("listed what the app does not hold: %s", b)
	}
	if h := refs["pkg:npm/react@19.3.1"].Hashes; len(h) != 1 || h[0].Alg != "SHA-512" || h[0].Content != "deadbeef" {
		t.Errorf("hashes = %+v", h)
	}
	if p := refs["pkg:golang/example.com/new@v1.1.0"].Properties; len(p) != 1 || p[0].Value != "h1:def=" {
		t.Errorf("properties = %+v", p)
	}
}

// NFR-SEC-004: the same build gives the same bill, whatever the map order.
func TestBillIsStable(t *testing.T) {
	first, _ := Build(App{Name: "Freeloop", Version: "0.0.1"}, info(), []byte(lock))
	for range 20 {
		b, err := Build(App{Name: "Freeloop", Version: "0.0.1"}, info(), []byte(lock))
		if err != nil || !bytes.Equal(b, first) {
			t.Fatalf("the bill changed: %v", err)
		}
	}
}

func TestBadInput(t *testing.T) {
	if _, err := Build(App{}, nil, []byte(lock)); err == nil {
		t.Error("no build information was accepted")
	}
	if _, err := Build(App{}, info(), []byte(`{`)); err == nil {
		t.Error("a broken lockfile was accepted")
	}
	weak := `{"packages": {"node_modules/x": {"version": "1.0.0", "integrity": "sha1-3q2+7w=="}}}`
	if _, err := Build(App{}, info(), []byte(weak)); err == nil {
		t.Error("a SHA-1 hash was accepted")
	}
	none := `{"packages": {"node_modules/x": {"version": "1.0.0"}}}`
	if _, err := Build(App{}, info(), []byte(none)); err == nil {
		t.Error("a package with no hash was accepted")
	}
}
