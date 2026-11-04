package secrets

import (
	"errors"
	"slices"
	"testing"
)

// itemStore is a Keychain with one item per reference, that can fail on demand.
type itemStore struct {
	*Memory
	accounts   []string
	gets, sets int
	denied     map[string]bool
	// corruptWrites makes the vault read back differently from what was written.
	corruptWrites bool
	failWrite     bool
	failDelete    bool
	failList      bool
	deleted       []string
}

func newItems(items map[string]string) *itemStore {
	s := &itemStore{Memory: NewMemory(), denied: map[string]bool{}}
	for account, value := range items {
		s.accounts = append(s.accounts, account)
		if err := s.Memory.Set(Ref{"app", account}, value); err != nil {
			panic(err)
		}
	}
	slices.Sort(s.accounts)
	return s
}

var errDenied = errors.New("denied")

func (s *itemStore) Get(ref Ref) (string, error) {
	s.gets++
	if s.denied[ref.Account] {
		return "", errDenied
	}
	return s.Memory.Get(ref)
}

func (s *itemStore) Set(ref Ref, value string) error {
	s.sets++
	if s.failWrite {
		return errors.New("write failed")
	}
	if s.corruptWrites {
		value = `{"version":1,"secrets":{}}`
	}
	if !slices.Contains(s.accounts, ref.Account) {
		s.accounts = append(s.accounts, ref.Account)
	}
	return s.Memory.Set(ref, value)
}

func (s *itemStore) Delete(ref Ref) error {
	if s.failDelete {
		return errors.New("delete failed")
	}
	s.deleted = append(s.deleted, ref.Account)
	s.accounts = slices.DeleteFunc(s.accounts, func(a string) bool { return a == ref.Account })
	return s.Memory.Delete(ref)
}

func (s *itemStore) Accounts(service string) ([]string, error) {
	if s.failList {
		return nil, errors.New("list failed")
	}
	if service != "app" {
		return nil, nil
	}
	return slices.Clone(s.accounts), nil
}

func want(t *testing.T, v *Vault, account, value string) {
	t.Helper()
	got, err := v.Get(Ref{"app", account})
	if err != nil || got != value {
		t.Fatalf("%s: got %q, %v, want %q", account, got, err, value)
	}
}

func TestVaultStoresAndForgets(t *testing.T) {
	v := NewVault(newItems(nil), "app")
	ref := Ref{"app", "providers/x"}
	if _, err := v.Get(ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty: %v", err)
	}
	if err := v.Set(ref, "key"); err != nil {
		t.Fatal(err)
	}
	want(t, v, "providers/x", "key")
	if err := v.Delete(ref); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Get(ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted: %v", err)
	}
	if err := v.Delete(ref); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted twice: %v", err)
	}
}

func TestVaultMovesItemsInAndRemovesThem(t *testing.T) {
	s := newItems(map[string]string{"a": "1", "b/c": "2", "bin": "\xff\x00"})
	v := NewVault(s, "app")
	want(t, v, "a", "1")
	want(t, v, "b/c", "2")
	want(t, v, "bin", "\xff\x00")
	if !slices.Equal(s.accounts, []string{VaultAccount}) {
		t.Fatalf("left in the store: %v", s.accounts)
	}
	// A new start finds them all in one item, with one read.
	s.gets = 0
	again := NewVault(s, "app")
	want(t, again, "a", "1")
	want(t, again, "bin", "\xff\x00")
	if s.gets != 1 {
		t.Fatalf("%d reads after the move, want 1", s.gets)
	}
}

func TestVaultReadsOnlyOnceInARun(t *testing.T) {
	s := newItems(map[string]string{"a": "1"})
	v := NewVault(s, "app")
	want(t, v, "a", "1")
	s.gets = 0
	want(t, v, "a", "1")
	if s.gets != 0 {
		t.Fatalf("%d reads", s.gets)
	}
}

func TestVaultKeepsAnItemTheUserDenied(t *testing.T) {
	s := newItems(map[string]string{"a": "1", "b": "2"})
	s.denied["b"] = true
	v := NewVault(s, "app")
	want(t, v, "a", "1")
	if _, err := v.Get(Ref{"app", "b"}); !errors.Is(err, errDenied) {
		t.Fatalf("denied item: %v", err)
	}
	if !slices.Contains(s.accounts, "b") || slices.Contains(s.deleted, "b") {
		t.Fatalf("the denied item is gone: %v", s.accounts)
	}
	// Allowed at the next start, it moves in.
	s.denied["b"] = false
	again := NewVault(s, "app")
	want(t, again, "a", "1")
	want(t, again, "b", "2")
	if slices.Contains(s.accounts, "b") {
		t.Fatalf("not removed after moving: %v", s.accounts)
	}
}

func TestVaultRemovesNothingUnlessItReadsBack(t *testing.T) {
	for name, breakIt := range map[string]func(*itemStore){
		"the write fails":     func(s *itemStore) { s.failWrite = true },
		"it reads back wrong": func(s *itemStore) { s.corruptWrites = true },
	} {
		t.Run(name, func(t *testing.T) {
			s := newItems(map[string]string{"a": "1", "b": "2"})
			breakIt(s)
			v := NewVault(s, "app")
			if _, err := v.Get(Ref{"app", "a"}); err == nil {
				t.Fatal("no error")
			}
			if len(s.deleted) != 0 {
				t.Fatalf("removed %v", s.deleted)
			}
			for _, account := range []string{"a", "b"} {
				if got, _ := s.Memory.Get(Ref{"app", account}); got == "" {
					t.Fatalf("%s is gone", account)
				}
			}
		})
	}
}

func TestVaultFinishesRemovingAfterADeleteFailed(t *testing.T) {
	s := newItems(map[string]string{"a": "1"})
	s.failDelete = true
	v := NewVault(s, "app")
	want(t, v, "a", "1")
	if !slices.Contains(s.accounts, "a") {
		t.Fatal("the delete should have failed")
	}
	s.failDelete = false
	want(t, NewVault(s, "app"), "a", "1")
	if slices.Contains(s.accounts, "a") {
		t.Fatalf("still there: %v", s.accounts)
	}
}

// A vault that cannot be read must not be replaced by an empty one: that is every secret.
func TestVaultNeverOverwritesWhatItCannotRead(t *testing.T) {
	denied := newItems(nil)
	v := NewVault(denied, "app")
	if err := v.Set(Ref{"app", "a"}, "1"); err != nil {
		t.Fatal(err)
	}
	denied.denied[VaultAccount] = true
	denied.sets = 0
	v2 := NewVault(denied, "app")
	if err := v2.Set(Ref{"app", "b"}, "2"); !errors.Is(err, errDenied) {
		t.Fatalf("set with the vault denied: %v", err)
	}
	if _, err := v2.Get(Ref{"app", "a"}); !errors.Is(err, errDenied) {
		t.Fatalf("get with the vault denied: %v", err)
	}
	if denied.sets != 0 {
		t.Fatalf("%d writes", denied.sets)
	}

	corrupt := newItems(map[string]string{VaultAccount: "not json", "b": "2"})
	v3 := NewVault(corrupt, "app")
	if err := v3.Set(Ref{"app", "c"}, "3"); err == nil {
		t.Fatal("set on a damaged vault")
	}
	if got, _ := corrupt.Memory.Get(Ref{"app", VaultAccount}); got != "not json" {
		t.Fatalf("the damaged vault became %q", got)
	}
	if got, _ := corrupt.Memory.Get(Ref{"app", "b"}); got != "2" {
		t.Fatal("an item was removed")
	}

	listing := newItems(map[string]string{"a": "1"})
	listing.failList = true
	if _, err := NewVault(listing, "app").Get(Ref{"app", "a"}); err == nil {
		t.Fatal("a failed listing passed as nothing there")
	}
	// It reads again when asked again, so a moment's failure does not stick.
	listing.failList = false
	want(t, NewVault(listing, "app"), "a", "1")
}

func TestVaultRefusesToHandOutItself(t *testing.T) {
	s := newItems(nil)
	v := NewVault(s, "app")
	if err := v.Set(Ref{"app", "a"}, "secret"); err != nil {
		t.Fatal(err)
	}
	ref := Ref{"app", VaultAccount}
	if got, err := v.Get(ref); !errors.Is(err, ErrNotFound) || got != "" {
		t.Fatalf("got %q, %v", got, err)
	}
	if v.Set(ref, "x") == nil || v.Delete(ref) == nil {
		t.Fatal("the vault item can be changed by reference")
	}
	want(t, v, "a", "secret")
}

func TestVaultLeavesOtherServicesAlone(t *testing.T) {
	s := newItems(nil)
	v := NewVault(s, "app")
	ref := Ref{"other", VaultAccount}
	if err := v.Set(ref, "x"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Memory.Get(ref); err != nil || got != "x" {
		t.Fatalf("not stored directly: %q, %v", got, err)
	}
	if got, err := v.Get(ref); err != nil || got != "x" {
		t.Fatalf("%q, %v", got, err)
	}
	if err := v.Delete(ref); err != nil {
		t.Fatal(err)
	}
}

// Another copy of the app, a development build say, shares the Keychain.
func TestVaultKeepsWhatAnotherCopyWrote(t *testing.T) {
	s := newItems(nil)
	first, second := NewVault(s, "app"), NewVault(s, "app")
	if err := first.Set(Ref{"app", "a"}, "1"); err != nil {
		t.Fatal(err)
	}
	if err := second.Set(Ref{"app", "b"}, "2"); err != nil {
		t.Fatal(err)
	}
	if err := first.Set(Ref{"app", "c"}, "3"); err != nil {
		t.Fatal(err)
	}
	fresh := NewVault(s, "app")
	want(t, fresh, "a", "1")
	want(t, fresh, "b", "2")
	want(t, fresh, "c", "3")
}
