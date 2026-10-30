package config

import (
	"strings"
	"testing"
)

func TestParseDevcontainer(t *testing.T) {
	d, err := ParseDevcontainer(".devcontainer/devcontainer.json", []byte(`{
	  // The dev image.
	  "name": "app",
	  "image": "mcr.microsoft.com/devcontainers/go:1-bookworm", /* pinned */
	  "postCreateCommand": ["go", "mod", "download"],
	  "containerEnv": {"URL": "http://example.com//x"},
	}`))
	if err != nil || d.Image != "mcr.microsoft.com/devcontainers/go:1-bookworm" || d.PostCreate != "'go' 'mod' 'download'" || d.Env["URL"] != "http://example.com//x" {
		t.Fatalf("parsed %+v, %v", d, err)
	}
	d, err = ParseDevcontainer(".devcontainer/devcontainer.json", []byte(`{"build": {"dockerfile": "Dockerfile", "context": ".."}, "postCreateCommand": "make setup"}`))
	if err != nil || d.Dockerfile != ".devcontainer/Dockerfile" || d.Context != "." || d.PostCreate != "make setup" {
		t.Errorf("build = %+v, %v", d, err)
	}
	if _, err := ParseDevcontainer("x.json", []byte(`{"dockerComposeFile": "c.yml"}`)); err == nil {
		t.Error("a compose devcontainer was accepted")
	}
}

func TestDevcontainerBuildStaysInProject(t *testing.T) {
	for _, b := range []string{
		`{"build": {"dockerfile": "Dockerfile", "context": "../../.."}}`,
		`{"build": {"dockerfile": "../../Dockerfile"}}`,
	} {
		if _, err := ParseDevcontainer(".devcontainer/devcontainer.json", []byte(b)); err == nil || !strings.Contains(err.Error(), "outside the project") {
			t.Errorf("%s: err = %v", b, err)
		}
	}
	d, err := ParseDevcontainer(".devcontainer/devcontainer.json", []byte(`{"build": {"dockerfile": "Dockerfile", "context": ".."}}`))
	if err != nil || d.Context != "." {
		t.Errorf("the project root as context = %+v, %v", d, err)
	}
}
