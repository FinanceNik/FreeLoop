// Package sshtest runs a real sshd for tests of SSH environments.
package sshtest

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Server is a real sshd on a free port, with a config for it as the user's own
// ~/.ssh/config would be: Host box connects with a known key, and Host unknownbox has a
// host key this Mac does not know.
type Server struct {
	Dir, Config string
}

// Start starts one for the test, or skips it where sshd cannot run.
func Start(t *testing.T) Server {
	t.Helper()
	const sshd = "/usr/sbin/sshd"
	if _, err := os.Stat(sshd); err != nil {
		t.Skip("no sshd")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"host", "user"} {
		if out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", filepath.Join(dir, name)).CombinedOutput(); err != nil {
			t.Fatalf("ssh-keygen: %v %s", err, out)
		}
	}
	pub, _ := os.ReadFile(filepath.Join(dir, "user.pub"))
	os.WriteFile(filepath.Join(dir, "auth"), pub, 0o600)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	cmd := exec.Command(sshd, "-D", "-e", "-p", fmt.Sprint(port), "-h", filepath.Join(dir, "host"),
		"-o", "AuthorizedKeysFile="+filepath.Join(dir, "auth"), "-o", "StrictModes=no", "-o", "PidFile="+filepath.Join(dir, "pid"),
		"-o", "UsePAM=no", "-o", "ListenAddress=127.0.0.1", "-o", "AllowAgentForwarding=yes", "-o", "AllowTcpForwarding=yes")
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Skipf("sshd: %v", err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	for i := 0; ; i++ {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		if err == nil {
			c.Close()
			break
		}
		if i > 50 {
			t.Skipf("sshd did not start: %s", logs.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	hostPub, _ := os.ReadFile(filepath.Join(dir, "host.pub"))
	known := filepath.Join(dir, "known")
	os.WriteFile(known, []byte(fmt.Sprintf("[127.0.0.1]:%d %s", port, hostPub)), 0o600)
	config := filepath.Join(dir, "config")
	os.WriteFile(config, []byte(fmt.Sprintf(`Host box
  HostName 127.0.0.1
  Port %d
  IdentityFile %s
  IdentitiesOnly yes
  UserKnownHostsFile %s
  # None of these may take effect: the command line wins.
  ForwardAgent yes
  StrictHostKeyChecking no
  PermitLocalCommand yes
  LocalCommand touch %s
Host unknownbox
  HostName 127.0.0.1
  Port %d
  IdentityFile %s
  IdentitiesOnly yes
  UserKnownHostsFile /dev/null
`, port, filepath.Join(dir, "user"), known, filepath.Join(dir, "local-command-ran"), port, filepath.Join(dir, "user"))), 0o600)
	return Server{Dir: dir, Config: config}
}
