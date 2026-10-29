package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Devcontainer is the part of a project's devcontainer.json a Docker
// environment uses (ENV-005): an image, or a Dockerfile to build, and the
// commands to run after the container is created.
type Devcontainer struct {
	Image string
	// Dockerfile and Context are relative to the folder of the
	// devcontainer.json.
	Dockerfile string
	Context    string
	PostCreate string
	Env        map[string]string
}

var (
	blockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	trailing     = regexp.MustCompile(`,(\s*[}\]])`)
)

// jsonc strips the comments and trailing commas devcontainer.json allows.
func jsonc(b []byte) []byte {
	s := blockComment.ReplaceAllString(string(b), "")
	var out []string
	for _, line := range strings.Split(s, "\n") {
		out = append(out, stripLineComment(line))
	}
	return []byte(trailing.ReplaceAllString(strings.Join(out, "\n"), "$1"))
}

// stripLineComment drops a // comment outside strings.
func stripLineComment(line string) string {
	inString, escaped := false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case escaped:
			escaped = false
		case c == '\\' && inString:
			escaped = true
		case c == '"':
			inString = !inString
		case !inString && c == '/' && i+1 < len(line) && line[i+1] == '/':
			return line[:i]
		}
	}
	return line
}

// ParseDevcontainer reads a devcontainer.json at file (a path relative to
// the project root).
func ParseDevcontainer(file string, b []byte) (Devcontainer, error) {
	var raw struct {
		Image string `json:"image"`
		Build struct {
			Dockerfile string `json:"dockerfile"`
			Context    string `json:"context"`
		} `json:"build"`
		DockerFile        string            `json:"dockerFile"`
		PostCreateCommand json.RawMessage   `json:"postCreateCommand"`
		ContainerEnv      map[string]string `json:"containerEnv"`
	}
	if err := json.Unmarshal(jsonc(b), &raw); err != nil {
		return Devcontainer{}, fmt.Errorf("%s: %w", file, err)
	}
	dir := path.Dir(file)
	d := Devcontainer{Image: raw.Image, Env: raw.ContainerEnv}
	if df := raw.Build.Dockerfile + raw.DockerFile; df != "" {
		d.Dockerfile = path.Join(dir, df)
		d.Context = dir
		if raw.Build.Context != "" {
			d.Context = path.Join(dir, raw.Build.Context)
		}
	}
	if d.Image == "" && d.Dockerfile == "" {
		return Devcontainer{}, fmt.Errorf("%s names neither an image nor a Dockerfile; Freeloop does not run Docker Compose devcontainers", file)
	}
	if len(raw.PostCreateCommand) > 0 {
		var one string
		var many []string
		var named map[string]json.RawMessage
		switch {
		case json.Unmarshal(raw.PostCreateCommand, &one) == nil:
			d.PostCreate = one
		case json.Unmarshal(raw.PostCreateCommand, &many) == nil:
			d.PostCreate = shellJoin(many)
		case json.Unmarshal(raw.PostCreateCommand, &named) == nil:
			var parts []string
			for _, v := range named {
				if json.Unmarshal(v, &one) == nil {
					parts = append(parts, one)
				} else if json.Unmarshal(v, &many) == nil {
					parts = append(parts, shellJoin(many))
				}
			}
			d.PostCreate = strings.Join(parts, " && ")
		default:
			return Devcontainer{}, errors.New(file + ": postCreateCommand is neither a string nor a list")
		}
	}
	return d, nil
}

// shellJoin quotes argv for sh.
func shellJoin(argv []string) string {
	var out []string
	for _, a := range argv {
		out = append(out, "'"+strings.ReplaceAll(a, "'", `'\''`)+"'")
	}
	return strings.Join(out, " ")
}
