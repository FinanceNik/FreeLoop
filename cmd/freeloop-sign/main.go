// freeloop-sign makes the project's release key and signs a release with it
// (SEC-021). It is a tool make release runs on the build machine and is not
// part of the app.
//
//	freeloop-sign keygen -out <file>      write a new signing key (never over one), print its public half
//	freeloop-sign public                  print the public half of the signing key
//	freeloop-sign sign -version <v> -dir <release folder>
//	                                      write SHA256SUMS.sig beside SHA256SUMS
//	freeloop-sign verify -version <v> -dir <release folder> [-key <public key>]
//	                                      check SHA256SUMS.sig and every file's checksum
//
// The signing key is read from RELEASE_SIGNING_KEY (the key itself, as CI has
// it in a secret) or from the file RELEASE_SIGNING_KEY_FILE names.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/FinanceNik/Freeloop/internal/release"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "freeloop-sign:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: freeloop-sign keygen|public|sign|verify (see the top of cmd/freeloop-sign/main.go)")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	outFile := fs.String("out", "", "the file to write the new key to")
	version := fs.String("version", "", "the release's version")
	dir := fs.String("dir", "", "the release's folder")
	pub := fs.String("key", "", "a public key to check with, in place of the app's own")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	switch args[0] {
	case "keygen":
		return keygen(*outFile, out)
	case "public":
		key, err := signingKey()
		if err != nil {
			return err
		}
		fmt.Fprintln(out, public(key))
		return nil
	case "sign":
		return sign(*version, *dir)
	case "verify":
		keys := release.Keys
		if *pub != "" {
			keys = []string{*pub}
		}
		return verify(keys, *version, *dir, out)
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func public(key ed25519.PrivateKey) string {
	return base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
}

// keygen writes a new key, readable by its owner only, and never over a file:
// a key that is replaced by accident leaves every installed copy unable to
// update.
func keygen(path string, out io.Writer) error {
	if path == "" {
		return errors.New("keygen needs -out <file>")
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, base64.StdEncoding.EncodeToString(key.Seed())); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Fprintln(out, public(key))
	return nil
}

func signingKey() (ed25519.PrivateKey, error) {
	if v := os.Getenv("RELEASE_SIGNING_KEY"); v != "" {
		return release.ParsePrivateKey(v)
	}
	path := os.Getenv("RELEASE_SIGNING_KEY_FILE")
	if path == "" {
		return nil, errors.New("no signing key: set RELEASE_SIGNING_KEY or RELEASE_SIGNING_KEY_FILE")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return release.ParsePrivateKey(string(b))
}

func sign(version, dir string) error {
	if version == "" || dir == "" {
		return errors.New("sign needs -version and -dir")
	}
	key, err := signingKey()
	if err != nil {
		return err
	}
	// An app only takes a release signed with a key it carries: signing with
	// another would make a release no installed copy accepts.
	if len(release.Keys) > 0 && !contains(release.Keys, public(key)) {
		return errors.New("the signing key is not one of release.Keys, so no copy of the app would accept this release")
	}
	sums, err := os.ReadFile(filepath.Join(dir, release.SumsName))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, release.SigName), []byte(release.Sign(key, version, sums)), 0o644)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if strings.TrimSpace(v) == s {
			return true
		}
	}
	return false
}

// verify checks a release folder as the app checks a download: the signature
// over the version and the list, then each listed file against the list.
func verify(keys []string, version, dir string, out io.Writer) error {
	if version == "" || dir == "" {
		return errors.New("verify needs -version and -dir")
	}
	sums, err := os.ReadFile(filepath.Join(dir, release.SumsName))
	if err != nil {
		return err
	}
	sig, err := os.ReadFile(filepath.Join(dir, release.SigName))
	if err != nil {
		return err
	}
	if err := release.Verify(keys, version, sums, string(sig)); err != nil {
		return err
	}
	list, err := release.Sums(sums)
	if err != nil {
		return err
	}
	for name, want := range list {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		if hex.EncodeToString(h.Sum(nil)) != want {
			return fmt.Errorf("%s is not the file the release lists", name)
		}
	}
	fmt.Fprintf(out, "release %s: signed by the project, %d files as listed\n", version, len(list))
	return nil
}
