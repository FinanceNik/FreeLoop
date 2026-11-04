package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"sync"
)

// VaultAccount is the account of the one item a Vault keeps its secrets in. A reference
// from a config file can name it, so it is refused: it would hand out every secret.
const VaultAccount = "vault"

// Lister is a Store that can say which accounts it holds under a service, without reading
// any of them. A Vault needs it to take over items stored one by one.
type Lister interface {
	Accounts(service string) ([]string, error)
}

// Vault keeps a service's secrets together in one item of the underlying store. macOS
// asks for permission per item and per build of an ad-hoc signed app, so one item means
// one prompt after a rebuild instead of one for each secret. Secrets of other services
// go straight through.
//
// Anything already stored item by item moves in on first use: it is written to the vault
// and read back, and only then removed, so a failure at any point loses nothing. A vault
// that cannot be read is never overwritten: a denied prompt or a damaged item is an error,
// not an empty vault.
type Vault struct {
	base    Store
	service string

	mu     sync.Mutex
	loaded bool
	values map[string][]byte
	// Items that could not be moved, such as one the user denied. They stay where they
	// are and move at the next start.
	failed map[string]error
}

// NewVault returns a Vault over base for service.
func NewVault(base Store, service string) *Vault {
	return &Vault{base: base, service: service}
}

var _ Store = (*Vault)(nil)

type vaultFile struct {
	Version int               `json:"version"`
	Secrets map[string][]byte `json:"secrets"`
}

func (v *Vault) ref() Ref { return Ref{Service: v.service, Account: VaultAccount} }

// read returns what is in the vault item; found is false when there is no such item.
func (v *Vault) read() (values map[string][]byte, found bool, err error) {
	raw, err := v.base.Get(v.ref())
	if errors.Is(err, ErrNotFound) {
		return map[string][]byte{}, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("secrets: read the vault: %w", err)
	}
	var f vaultFile
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		return nil, true, fmt.Errorf("secrets: the vault %s cannot be read, and is left as it is: %w", v.ref(), err)
	}
	if f.Secrets == nil {
		f.Secrets = map[string][]byte{}
	}
	return f.Secrets, true, nil
}

func (v *Vault) write(values map[string][]byte) error {
	raw, err := json.Marshal(vaultFile{Version: 1, Secrets: values})
	if err != nil {
		return err
	}
	return v.base.Set(v.ref(), string(raw))
}

// load reads the vault once and moves in what is stored item by item. Call it with mu held.
func (v *Vault) load() error {
	if v.loaded {
		return nil
	}
	values, _, err := v.read()
	if err != nil {
		return err
	}
	failed := map[string]error{}
	if lister, ok := v.base.(Lister); ok {
		accounts, err := lister.Accounts(v.service)
		if err != nil {
			return fmt.Errorf("secrets: list %s: %w", v.service, err)
		}
		if err := v.moveIn(accounts, values, failed); err != nil {
			return err
		}
	}
	v.values, v.failed, v.loaded = values, failed, true
	return nil
}

// moveIn takes over the items in accounts, and removes each once the vault holds it. An
// item the vault already has is a leftover of a move that could not finish removing it.
func (v *Vault) moveIn(accounts []string, values map[string][]byte, failed map[string]error) error {
	var moved, leftover []string
	for _, account := range accounts {
		if account == VaultAccount {
			continue
		}
		if _, ok := values[account]; ok {
			leftover = append(leftover, account)
			continue
		}
		value, err := v.base.Get(Ref{Service: v.service, Account: account})
		if err != nil {
			log.Printf("secrets: %s stays as a separate item for now: %v", account, err)
			failed[account] = err
			continue
		}
		values[account] = []byte(value)
		moved = append(moved, account)
	}
	if len(moved) > 0 {
		if err := v.write(values); err != nil {
			return fmt.Errorf("secrets: write the vault: %w", err)
		}
		// Back from the store, not from memory, before anything is removed.
		back, _, err := v.read()
		if err != nil {
			return err
		}
		if !maps.EqualFunc(back, values, func(a, b []byte) bool { return string(a) == string(b) }) {
			return errors.New("secrets: the vault did not read back as written; the separate items are kept")
		}
	}
	for _, account := range append(moved, leftover...) {
		if err := v.base.Delete(Ref{Service: v.service, Account: account}); err != nil && !errors.Is(err, ErrNotFound) {
			log.Printf("secrets: the old item %s stays until the next start: %v", account, err)
		}
	}
	return nil
}

func (v *Vault) Get(ref Ref) (string, error) {
	if ref.Service != v.service {
		return v.base.Get(ref)
	}
	if ref.Account == VaultAccount {
		return "", ErrNotFound
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.load(); err != nil {
		return "", err
	}
	if value, ok := v.values[ref.Account]; ok {
		return string(value), nil
	}
	if err, ok := v.failed[ref.Account]; ok {
		return "", err
	}
	return "", ErrNotFound
}

// change applies an edit to what the store holds now, since another copy of the app may
// have written since this one read.
func (v *Vault) change(edit func(map[string][]byte) error) error {
	if err := v.load(); err != nil {
		return err
	}
	values, _, err := v.read()
	if err != nil {
		return err
	}
	if err := edit(values); err != nil {
		return err
	}
	if err := v.write(values); err != nil {
		return err
	}
	v.values = values
	return nil
}

func (v *Vault) Set(ref Ref, value string) error {
	if ref.Service != v.service {
		return v.base.Set(ref, value)
	}
	if ref.Account == VaultAccount {
		return fmt.Errorf("secrets: %s is the vault itself", ref)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.change(func(values map[string][]byte) error {
		values[ref.Account] = []byte(value)
		return nil
	})
}

func (v *Vault) Delete(ref Ref) error {
	if ref.Service != v.service {
		return v.base.Delete(ref)
	}
	if ref.Account == VaultAccount {
		return fmt.Errorf("secrets: %s is the vault itself", ref)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if err := v.load(); err != nil {
		return err
	}
	// One that could not be moved is still a separate item.
	if _, ok := v.failed[ref.Account]; ok {
		delete(v.failed, ref.Account)
		return v.base.Delete(ref)
	}
	return v.change(func(values map[string][]byte) error {
		if _, ok := values[ref.Account]; !ok {
			return ErrNotFound
		}
		delete(values, ref.Account)
		return nil
	})
}
