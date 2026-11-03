package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/netguard"
)

// Browsing an MCP server registry (MCP-009). The address is the user's to set in
// Settings, so nothing connects until then (the only outbound traffic is to
// endpoints the user configured). A result only fills in the add-server form: it
// is text from strangers, and the server starts under the same confirmation of
// its exact command as one typed by hand (MCP-005).

// OfficialMCPRegistry is what Settings offers to fill in.
const OfficialMCPRegistry = "https://registry.modelcontextprotocol.io"

// RegistryServer is one server of a registry, as a draft for the add-server form.
type RegistryServer struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Repository  string `json:"repository"`
	// Options are the ways to run it this app can use.
	Options []RegistryOption `json:"options"`
	// Unsupported says what else it offers that the app cannot run.
	Unsupported []string `json:"unsupported"`
}

// RegistryOption is one way to run a server.
type RegistryOption struct {
	Label     string `json:"label"`
	Transport string `json:"transport"` // stdio | http
	Command   string `json:"command"`
	// Args always pin the version the registry lists.
	Args []string `json:"args"`
	URL  string   `json:"url"`
	// Variables and Headers are what it asks to be given. Secret ones go to the
	// Keychain through the form, as any other.
	Variables []RegistryValue `json:"variables"`
	Headers   []RegistryValue `json:"headers"`
}

// RegistryValue is a variable or header a server asks for.
type RegistryValue struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"secret"`
	Default     string `json:"default"`
}

// RegistryPage is one page of results.
type RegistryPage struct {
	Servers []RegistryServer `json:"servers"`
	Next    string           `json:"next"`
}

type registryValue struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsRequired  bool   `json:"isRequired"`
	IsSecret    bool   `json:"isSecret"`
	Default     string `json:"default"`
}

type registryResponse struct {
	Servers []struct {
		Server struct {
			Name        string `json:"name"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Version     string `json:"version"`
			Repository  struct {
				URL string `json:"url"`
			} `json:"repository"`
			Packages []struct {
				RegistryType string `json:"registryType"`
				Identifier   string `json:"identifier"`
				Version      string `json:"version"`
				Transport    struct {
					Type string `json:"type"`
				} `json:"transport"`
				RuntimeArguments []registryArgument `json:"runtimeArguments"`
				PackageArguments []registryArgument `json:"packageArguments"`
				Environment      []registryValue    `json:"environmentVariables"`
			} `json:"packages"`
			Remotes []struct {
				Type    string          `json:"type"`
				URL     string          `json:"url"`
				Headers []registryValue `json:"headers"`
			} `json:"remotes"`
		} `json:"server"`
	} `json:"servers"`
	Metadata struct {
		NextCursor string `json:"nextCursor"`
	} `json:"metadata"`
}

type registryArgument struct {
	Type       string `json:"type"`
	Name       string `json:"name"`
	Value      string `json:"value"`
	IsRequired bool   `json:"isRequired"`
}

// A package identifier and version become a command line, so only plain ones do.
var (
	registryPackage = regexp.MustCompile(`^(@[a-zA-Z0-9._-]+/)?[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	// It starts with a digit: "latest" is not a pinned version.
	registryVersion = regexp.MustCompile(`^v?[0-9][a-zA-Z0-9.+_-]*$`)
)

// maxRegistryPage bounds what one page may be.
const maxRegistryPage = 8 << 20

// SearchMCPRegistry returns a page of servers matching query, from the registry
// the user set.
func (e *Engine) SearchMCPRegistry(ctx context.Context, query, cursor string) (RegistryPage, error) {
	base := strings.TrimRight(e.Config().MCP.RegistryURL, "/")
	if base == "" {
		return RegistryPage{}, errors.New("set a registry address in Settings first")
	}
	if netguard.LocalOnly() {
		return RegistryPage{}, errors.New("the app is set to connect to nothing but this Mac")
	}
	if !registryAddress(base) {
		return RegistryPage{}, fmt.Errorf("%q is not an https address", base)
	}
	if !e.commandApproved(config.AddressFingerprint("mcp.registryURL", e.Config().MCP.RegistryURL)) {
		return RegistryPage{}, fmt.Errorf("%s has not been approved; approve it in Settings, or enter it here again", base)
	}
	u, err := url.Parse(base + "/v0.1/servers")
	if err != nil {
		return RegistryPage{}, err
	}
	q := url.Values{"limit": {"30"}, "version": {"latest"}}
	if s := strings.TrimSpace(query); s != "" {
		q.Set("search", s)
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	u.RawQuery = q.Encode()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return RegistryPage{}, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return RegistryPage{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return RegistryPage{}, fmt.Errorf("the registry answered %s", res.Status)
	}
	var raw registryResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, maxRegistryPage)).Decode(&raw); err != nil {
		return RegistryPage{}, fmt.Errorf("the registry's answer: %w", err)
	}
	page := RegistryPage{Servers: []RegistryServer{}, Next: raw.Metadata.NextCursor}
	for _, entry := range raw.Servers {
		s := entry.Server
		out := RegistryServer{Name: s.Name, Title: s.Title, Description: s.Description, Version: s.Version, Repository: s.Repository.URL, Options: []RegistryOption{}, Unsupported: []string{}}
		if !strings.HasPrefix(out.Repository, "https://") {
			out.Repository = "" // shown as a link
		}
		for _, p := range s.Packages {
			if p.Transport.Type != "" && p.Transport.Type != "stdio" {
				out.Unsupported = append(out.Unsupported, p.RegistryType+" over "+p.Transport.Type)
				continue
			}
			opt, ok := packageOption(p.RegistryType, p.Identifier, p.Version)
			if !ok {
				out.Unsupported = append(out.Unsupported, p.RegistryType+" package "+p.Identifier)
				continue
			}
			args, askable := registryArgs(p.PackageArguments)
			if !askable {
				out.Unsupported = append(out.Unsupported, p.RegistryType+" package "+p.Identifier+" with required arguments")
				continue
			}
			opt.Args = append(opt.Args, args...)
			opt.Variables = registryValues(p.Environment)
			out.Options = append(out.Options, opt)
		}
		for _, r := range s.Remotes {
			if r.Type != "streamable-http" || !strings.HasPrefix(r.URL, "https://") {
				out.Unsupported = append(out.Unsupported, r.Type+" at "+r.URL)
				continue
			}
			out.Options = append(out.Options, RegistryOption{Label: "Remote: " + r.URL, Transport: "http", URL: r.URL, Args: []string{}, Variables: []RegistryValue{}, Headers: registryValues(r.Headers)})
		}
		page.Servers = append(page.Servers, out)
	}
	return page, nil
}

// packageOption writes a package as a command, pinned to the version listed. It
// runs what the user's own tools fetch; nothing is installed here.
func packageOption(kind, id, version string) (RegistryOption, bool) {
	if !registryPackage.MatchString(id) || !registryVersion.MatchString(version) {
		return RegistryOption{}, false
	}
	opt := RegistryOption{Transport: "stdio", Variables: []RegistryValue{}, Headers: []RegistryValue{}}
	switch kind {
	case "npm":
		opt.Label, opt.Command, opt.Args = "npm: "+id, "npx", []string{"-y", id + "@" + version}
	case "pypi":
		opt.Label, opt.Command, opt.Args = "PyPI: "+id, "uvx", []string{id + "==" + version}
	default:
		return RegistryOption{}, false
	}
	return opt, true
}

// registryArgs takes the arguments a package always runs with. One that needs a
// value from the user cannot be filled in here.
func registryArgs(args []registryArgument) ([]string, bool) {
	out := []string{}
	for _, a := range args {
		switch {
		case a.Value != "" && a.Type == "named":
			out = append(out, a.Name, a.Value)
		case a.Value != "":
			out = append(out, a.Value)
		case a.IsRequired:
			return nil, false
		}
	}
	return out, true
}

func registryValues(in []registryValue) []RegistryValue {
	out := make([]RegistryValue, 0, len(in))
	for _, v := range in {
		// A value the registry gives for a secret is a template at best.
		def := v.Default
		if v.IsSecret {
			def = ""
		}
		out = append(out, RegistryValue{Name: v.Name, Description: v.Description, Required: v.IsRequired, Secret: v.IsSecret, Default: def})
	}
	slices.SortStableFunc(out, func(a, b RegistryValue) int {
		switch {
		case a.Required == b.Required:
			return 0
		case a.Required:
			return -1
		}
		return 1
	})
	return out
}

// MCPRegistryAddress is the registry the user set, if any.
func (e *Engine) MCPRegistryAddress() string { return e.Config().MCP.RegistryURL }

// SetMCPRegistryAddress keeps a registry's address; empty forgets it.
func (e *Engine) SetMCPRegistryAddress(address string) error {
	address = strings.TrimRight(strings.TrimSpace(address), "/")
	if address != "" && !registryAddress(address) {
		return fmt.Errorf("%q is not an https address", address)
	}
	c := e.Config()
	c.MCP.RegistryURL = address
	_, err := e.saveChoosing(c, config.AddressFingerprint("mcp.registryURL", address))
	return err
}

// registryAddress takes https, or http on this Mac for a registry of one's own.
func registryAddress(address string) bool {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))
}
