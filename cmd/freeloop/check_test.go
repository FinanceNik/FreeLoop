package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FinanceNik/Freeloop/internal/engine"
	"github.com/FinanceNik/Freeloop/internal/permission"
	"github.com/FinanceNik/Freeloop/internal/rpc"
	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// SEC-018: freeloop check prints the verdict, and its exit code tells allowed,
// asked and denied from an error.
func TestCheckCommand(t *testing.T) {
	e, err := engine.Open(engine.Options{DataDir: t.TempDir(), Secrets: secrets.NewMemory(), Home: t.TempDir(), Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := e.AddRule(permission.Rule{Effect: permission.Deny, Tool: "shell", Command: "git push *"}); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("/tmp", "fl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "s.sock")
	srv, err := rpc.Listen(e, sock)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	defer srv.Close()
	project := t.TempDir()

	run := func(args ...string) (int, string, string) {
		var out, errOut bytes.Buffer
		code := runCLI(sock, append([]string{"check", "-project", project}, args...), &out, &errOut)
		return code, out.String(), errOut.String()
	}
	for _, c := range []struct {
		args []string
		code int
		says string
	}{
		{[]string{"shell", "git", "push", "origin", "main"}, 3, "Denied: deny shell(git push *) (global rule)"},
		{[]string{"-mode", "bypass", "shell", "git push"}, 3, "in bypass mode"},
		{[]string{"shell", "make test"}, 2, "Asks you"},
		{[]string{"read", "main.go"}, 0, "Allowed: reads inside the project are allowed"},
		{[]string{"-mode", "plan", "edit", "main.go"}, 3, "plan mode"},
	} {
		code, out, errOut := run(c.args...)
		if code != c.code || !strings.Contains(out, c.says) {
			t.Errorf("%v: exit %d, %q %q; want %d with %q", c.args, code, out, errOut, c.code, c.says)
		}
	}
	code, out, _ := run("-json", "shell", "git push")
	var v engine.PolicyCheck
	if err := json.Unmarshal([]byte(out), &v); err != nil || code != 3 || v.Outcome != "denied" || v.Rule == nil {
		t.Errorf("-json: exit %d, %v, %s", code, err, out)
	}
	for _, bad := range [][]string{{"shell"}, {"-mode", "loose", "shell", "ls"}, {"-nope", "shell", "ls"}} {
		if code, _, errOut := run(bad...); code != 1 || errOut == "" {
			t.Errorf("%v: exit %d, %q", bad, code, errOut)
		}
	}
}
