package config

import "testing"

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
