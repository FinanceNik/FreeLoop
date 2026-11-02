package engine

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/config"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// A key is only as safe as the address it is sent to. The config files say that
// address, and they are plain files: whatever can write them, a command the user
// approved, an MCP server, a restored backup, another program, could point a
// provider at itself and have the app send the key, with no Keychain prompt,
// since the app is allowed to read its own items. So a key is kept in the
// Keychain together with the address the user gave it for, which a file cannot
// change, and is sent nowhere else until the user says so in the app.

// boundSecret is what a Keychain item holds for a key tied to an address. The
// address can be empty: a search key entered before its address is.
type boundSecret struct {
	Address *string `json:"freeloopAddress"`
	Key     string  `json:"key"`
}

func bindSecret(address, key string) string {
	b, _ := json.Marshal(boundSecret{Address: &address, Key: key})
	return string(b)
}

// unbindSecret splits an item's value. bound is false for one stored before keys
// had addresses, or by hand: the whole value is the key.
func unbindSecret(stored string) (address, key string, bound bool) {
	var s boundSecret
	if json.Unmarshal([]byte(stored), &s) == nil && s.Address != nil && s.Key != "" {
		return *s.Address, s.Key, true
	}
	return "", stored, false
}

// originOf is the scheme, host and port of an address: what decides who gets the
// key. A path on the same host is the same party, and userinfo is not the host.
func originOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return strings.TrimSpace(raw)
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}

// keyAddress names where a provider's key goes. An empty base URL means the
// default of its type and preset, so those are part of it.
func keyAddress(c config.Provider) string {
	return c.Type + "|" + c.Preset + "|" + originOf(c.BaseURL)
}

// providerRef is the one Keychain item a provider's key may be in. A file naming
// another item could have any secret sent as a key, a vendor's sign-in included.
func providerRef(id string) secrets.Ref {
	return secrets.Ref{Service: "freeloop", Account: "providers/" + id}
}

// ownRef parses a reference from a config file and refuses one that is not the
// item the app itself would use for this secret.
func ownRef(raw string, want secrets.Ref) (secrets.Ref, error) {
	ref, err := secrets.ParseRef(raw)
	if err != nil {
		return ref, err
	}
	if ref != want {
		return ref, fmt.Errorf("it refers to the Keychain item %s, not %s, and is not read", ref, want)
	}
	return ref, nil
}

// providerKey reads a provider's key if the provider still has the address the
// key is for. A key stored before keys had addresses takes the current one.
func (e *Engine) providerKey(c config.Provider) (string, error) {
	ref, err := ownRef(c.APIKey, providerRef(c.ID))
	if err != nil {
		return "", fmt.Errorf("provider %s: %w", c.Name, err)
	}
	stored, err := e.secrets.Get(ref)
	if err != nil {
		return "", fmt.Errorf("provider %s: read API key from the Keychain: %w", c.Name, err)
	}
	address, key, bound := unbindSecret(stored)
	want := keyAddress(c)
	if !bound {
		if err := e.secrets.Set(ref, bindSecret(want, key)); err != nil {
			return "", fmt.Errorf("provider %s: store API key in the Keychain: %w", c.Name, err)
		}
		address = want
	}
	e.noteKeyAddress(c.ID, address)
	e.remember(key)
	if address != want {
		return "", fmt.Errorf("provider %s: its address changed to %s outside Freeloop, so its API key is not sent there. If you made that change, confirm it under Providers", c.Name, displayAddress(c))
	}
	return key, nil
}

func displayAddress(c config.Provider) string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return "its default endpoint"
}

func (e *Engine) noteKeyAddress(id, address string) {
	e.mu.Lock()
	if e.keyAddresses == nil {
		e.keyAddresses = map[string]string{}
	}
	e.keyAddresses[id] = address
	e.mu.Unlock()
	e.checkKeyAddresses()
}

// checkKeyAddresses reports providers whose address differs from their key's.
func (e *Engine) checkKeyAddresses() {
	e.mu.Lock()
	defer e.mu.Unlock()
	var issues []config.Issue
	for _, c := range e.providers.configs {
		if a, ok := e.keyAddresses[c.ID]; ok && c.APIKey != "" && a != keyAddress(c) {
			issues = append(issues, config.Issue{File: config.ProvidersFileName, Field: c.ID,
				Message: fmt.Sprintf("the address changed to %s outside Freeloop, so the API key is not sent there; confirm it under Providers if you made that change", displayAddress(c))})
		}
	}
	e.issues[issueAddresses] = issues
}

// addressChanged reports a provider whose key is for another address.
func (e *Engine) addressChanged(c config.Provider) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	a, ok := e.keyAddresses[c.ID]
	return ok && c.APIKey != "" && a != keyAddress(c)
}

// ConfirmProviderAddress is the user saying, in the app, that a provider's
// current address is the one its key is for.
func (e *Engine) ConfirmProviderAddress(id string) error {
	c, err := e.providerConfig(id)
	if err != nil {
		return err
	}
	if c.APIKey == "" {
		return nil
	}
	ref, err := ownRef(c.APIKey, providerRef(c.ID))
	if err != nil {
		return err
	}
	stored, err := e.secrets.Get(ref)
	if err != nil {
		return fmt.Errorf("read API key from the Keychain: %w", err)
	}
	_, key, _ := unbindSecret(stored)
	if err := e.secrets.Set(ref, bindSecret(keyAddress(c), key)); err != nil {
		return fmt.Errorf("store API key in the Keychain: %w", err)
	}
	e.noteKeyAddress(c.ID, keyAddress(c))
	e.mu.Lock()
	delete(e.providers.built, c.ID)
	e.mu.Unlock()
	return nil
}

func (e *Engine) providerConfig(id string) (config.Provider, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, c := range e.providers.configs {
		if c.ID == id {
			return c, nil
		}
	}
	return config.Provider{}, fmt.Errorf("no provider %q", id)
}

// searchKey reads the search key if the search address is still the one it is
// for, the way providerKey does.
func (e *Engine) searchKey(searchURL string) (string, error) {
	stored, err := e.secrets.Get(webSearchKey)
	if err != nil {
		return "", fmt.Errorf("read the search key from the Keychain: %w", err)
	}
	address, key, bound := unbindSecret(stored)
	want := originOf(searchURL)
	if !bound {
		if err := e.secrets.Set(webSearchKey, bindSecret(want, key)); err != nil {
			return "", fmt.Errorf("store the search key in the Keychain: %w", err)
		}
		address = want
	}
	e.remember(key)
	if address != want {
		return "", fmt.Errorf("the search address changed to %s outside Freeloop, so the search key is not sent there. If you made that change, enter the key again in Settings", want)
	}
	return key, nil
}

// rebindSearchKey moves the search key to a new address the user set in the app.
func (e *Engine) rebindSearchKey(searchURL string) error {
	stored, err := e.secrets.Get(webSearchKey)
	if errors.Is(err, secrets.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	_, key, _ := unbindSecret(stored)
	return e.secrets.Set(webSearchKey, bindSecret(originOf(searchURL), key))
}

// approvalKeyRef holds the key that signs MCP approvals.
var approvalKeyRef = secrets.Ref{Service: "freeloop", Account: "app/approvals"}

// approvalMark is what mcp.yaml records for an approved server. A plain hash of
// the server's settings could be computed by whatever wrote the file, which would
// then have approved its own server, and with it any secret the server has; this
// needs a key only the app can read.
func (e *Engine) approvalMark(fingerprint string) (string, error) {
	key, err := e.approvalKey()
	if err != nil {
		return "", err
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte(fingerprint))
	return hex.EncodeToString(m.Sum(nil)), nil
}

// mcpApproved reports whether the user approved exactly this configuration.
func (e *Engine) mcpApproved(s config.MCPServer) bool {
	if s.Approved == "" {
		return false
	}
	mark, err := e.approvalMark(s.Fingerprint())
	return err == nil && hmac.Equal([]byte(mark), []byte(s.Approved))
}

func (e *Engine) approvalKey() ([]byte, error) {
	e.approvalOnce.Lock()
	defer e.approvalOnce.Unlock()
	if e.approvalSecret != nil {
		return e.approvalSecret, nil
	}
	stored, err := e.secrets.Get(approvalKeyRef)
	if errors.Is(err, secrets.ErrNotFound) {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		stored = hex.EncodeToString(b)
		if err := e.secrets.Set(approvalKeyRef, stored); err != nil {
			return nil, fmt.Errorf("store the approval key in the Keychain: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("read the approval key from the Keychain: %w", err)
	}
	e.approvalSecret = []byte(stored)
	return e.approvalSecret, nil
}

// credentialParam matches a query parameter that holds a credential.
var credentialParam = regexp.MustCompile(`(?i)^(.*[_-])?(key|apikey|token|secret|password|passwd|pwd|auth|sig|signature)$`)

// credentialsIn returns the credential-like values in an address's query and
// userinfo. A {key} or {query} placeholder is not one.
func credentialsIn(u *url.URL) []string {
	var out []string
	if u.User != nil {
		if p, ok := u.User.Password(); ok && p != "" {
			out = append(out, p)
		}
		if n := u.User.Username(); n != "" {
			out = append(out, n)
		}
	}
	for name, values := range u.Query() {
		if credentialParam.MatchString(name) {
			for _, v := range values {
				if v != "" && v != "{key}" && v != "{query}" {
					out = append(out, v)
				}
			}
		}
	}
	return out
}

// checkAddress refuses an address holding a credential: config files are not a
// place for one, a backup carries them, and it would show in errors.
func checkAddress(what, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("%s is not an address: %w", what, err)
	}
	if u.User != nil {
		return fmt.Errorf("%s holds a user name or password; those go in the Keychain, not in the address", what)
	}
	if len(credentialsIn(u)) > 0 {
		return fmt.Errorf("%s holds a key in its query; enter the key separately so it stays in the Keychain", what)
	}
	return nil
}
