// Package secretstest is a conformance suite every secrets.Store backend
// must pass.
package secretstest

import (
	"errors"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// Run exercises s with references under service. It deletes what it creates.
func Run(t *testing.T, s secrets.Store, service string) {
	t.Helper()
	ref := secrets.Ref{Service: service, Account: "secretstest/key"}
	s.Delete(ref) // leftovers from an aborted run

	if _, err := s.Get(ref); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("Get of missing secret: err = %v, want ErrNotFound", err)
	}
	if err := s.Set(ref, "first value ✓"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	t.Cleanup(func() { s.Delete(ref) })
	if v, err := s.Get(ref); err != nil || v != "first value ✓" {
		t.Fatalf("Get = %q, %v", v, err)
	}
	if err := s.Set(ref, "second"); err != nil {
		t.Fatalf("Set (overwrite): %v", err)
	}
	if v, err := s.Get(ref); err != nil || v != "second" {
		t.Fatalf("Get after overwrite = %q, %v", v, err)
	}
	if err := s.Delete(ref); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ref); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("Get after Delete: err = %v, want ErrNotFound", err)
	}
	if err := s.Delete(ref); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("second Delete: err = %v, want ErrNotFound", err)
	}
}
