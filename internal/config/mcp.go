package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// MCPFileName holds the MCP servers (MCP-001).
const MCPFileName = "mcp.yaml"

// MCPServer is one configured MCP server: a command started over stdio, or
// a Streamable HTTP endpoint. Secrets are keychain:// references resolved
// when the server starts.
type MCPServer struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name,omitempty" json:"name"`
	// Transport is stdio or http.
	Transport string            `yaml:"transport" json:"transport"`
	Command   string            `yaml:"command,omitempty" json:"command"`
	Args      []string          `yaml:"args,omitempty" json:"args"`
	Env       map[string]string `yaml:"env,omitempty" json:"env"`
	// Secrets maps a variable (stdio) or header (http) name to a keychain
	// reference.
	Secrets map[string]string `yaml:"secrets,omitempty" json:"-"`
	URL     string            `yaml:"url,omitempty" json:"url"`
	Headers map[string]string `yaml:"headers,omitempty" json:"headers"`
	// OAuthClientID is a client the server's authorization server knows
	// this app by; without one the app registers itself (MCP-006).
	OAuthClientID string `yaml:"oauthClientId,omitempty" json:"oauthClientId"`
	// Enabled offers the server's tools to every session; a profile can add
	// servers that are off here.
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Approved is the fingerprint of the configuration the user confirmed
	// (MCP-005); any change to what runs or where it connects asks again.
	Approved string `yaml:"approved,omitempty" json:"-"`
	// Source records where an imported server came from (MCP-002).
	Source string `yaml:"source,omitempty" json:"source"`
}

type mcpFile struct {
	Version int         `yaml:"version"`
	Servers []MCPServer `yaml:"servers"`
}

// Fingerprint hashes everything that decides what the server runs or where
// it connects, secret references included, values of secrets excluded.
func (s MCPServer) Fingerprint() string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00", s.Transport, s.Command, strings.Join(s.Args, "\x01"), s.URL)
	for _, m := range []map[string]string{s.Env, s.Secrets, s.Headers} {
		for _, k := range slices.Sorted(maps.Keys(m)) {
			fmt.Fprintf(h, "%s=%s\x01", k, m[k])
		}
		h.Write([]byte{0})
	}
	// Only when set, so servers approved before the field existed keep
	// their fingerprints.
	if s.OAuthClientID != "" {
		fmt.Fprintf(h, "oauth=%s\x00", s.OAuthClientID)
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// Validate reports a server that cannot work.
func (s MCPServer) Validate() error {
	switch {
	case !idPattern.MatchString(s.ID) || strings.Contains(s.ID, "--"):
		return fieldErr("id", "%q must be lowercase letters, digits and single dashes", s.ID)
	case s.Transport == "stdio" && strings.TrimSpace(s.Command) == "":
		return fieldErr("command", "a stdio server needs a command")
	case s.Transport == "http":
		u, err := url.Parse(s.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return fieldErr("url", "%q is not an http(s) URL", s.URL)
		}
	case s.Transport != "stdio":
		return fieldErr("transport", "%q must be stdio or http", s.Transport)
	}
	for k := range s.Env {
		if !varName.MatchString(k) {
			return fieldErr("env", "%q is not a valid variable name", k)
		}
	}
	for k, ref := range s.Secrets {
		if !strings.HasPrefix(ref, "keychain://") {
			return fieldErr("secrets", "%s must be a keychain:// reference, never a value", k)
		}
	}
	return nil
}

// LoadMCP reads mcp.yaml. Servers with problems are skipped and reported.
func LoadMCP(dir string) ([]MCPServer, []Issue, error) {
	path := filepath.Join(dir, MCPFileName)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	var f mcpFile
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return nil, []Issue{{File: path, Line: yamlErrorLine(err), Message: err.Error()}}, nil
	}
	var out []MCPServer
	var issues []Issue
	seen := map[string]bool{}
	for i, s := range f.Servers {
		if s.Transport == "" {
			s.Transport = "stdio"
		}
		field := fmt.Sprintf("servers[%d]", i)
		err := s.Validate()
		if err == nil && seen[s.ID] {
			err = fieldErr("id", "%s is defined twice", s.ID)
		}
		if err != nil {
			is := Issue{File: path, Field: field, Line: fieldLine(b, field), Message: err.Error()}
			issues = append(issues, is)
			continue
		}
		seen[s.ID] = true
		out = append(out, s)
	}
	return out, issues, nil
}

// SaveMCP replaces mcp.yaml.
func SaveMCP(dir string, servers []MCPServer) error {
	for _, s := range servers {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("%s: %w", s.ID, err)
		}
	}
	b, err := yaml.Marshal(mcpFile{Version: 1, Servers: servers})
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, MCPFileName), b)
}
