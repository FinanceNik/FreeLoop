// Package secrets defines the interface every secret goes through (SEC-004).
// Config files hold references such as keychain://freeloop/providers/anthropic;
// values are resolved only at request or spawn time. The backends are the system's
// keyrings: the macOS Keychain in internal/platform/darwin, the Secret Service in
// internal/platform/linux and the Credential Manager in internal/platform/windows;
// Memory is the fake for tests.
package secrets

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

// ErrNotFound is returned when no secret exists for a reference.
var ErrNotFound = errors.New("secrets: not found")

// Store reads and writes secret values by reference.
type Store interface {
	Get(ref Ref) (string, error)
	Set(ref Ref, value string) error
	Delete(ref Ref) error
}

// Scheme is the URL scheme of a secret reference.
const Scheme = "keychain"

// Ref identifies one secret: a Keychain generic password's service and account.
type Ref struct {
	Service string
	Account string
}

// ParseRef parses "keychain://<service>/<account>". The account may contain
// slashes.
func ParseRef(s string) (Ref, error) {
	rest, ok := strings.CutPrefix(s, Scheme+"://")
	if !ok {
		return Ref{}, fmt.Errorf("secrets: %q is not a %s:// reference", s, Scheme)
	}
	service, account, ok := strings.Cut(rest, "/")
	if !ok || service == "" || account == "" {
		return Ref{}, fmt.Errorf("secrets: %q must be %s://<service>/<account>", s, Scheme)
	}
	return Ref{Service: service, Account: account}, nil
}

func (r Ref) String() string {
	return Scheme + "://" + r.Service + "/" + r.Account
}

// Memory is an in-memory Store for tests.
type Memory struct {
	mu sync.Mutex
	m  map[Ref]string
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory {
	return &Memory{m: map[Ref]string{}}
}

func (m *Memory) Get(ref Ref) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.m[ref]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (m *Memory) Set(ref Ref, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[ref] = value
	return nil
}

func (m *Memory) Delete(ref Ref) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.m[ref]; !ok {
		return ErrNotFound
	}
	delete(m.m, ref)
	return nil
}
