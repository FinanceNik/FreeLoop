//go:build linux

package environment

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"syscall"
)

// RunSandboxBridge is the app's own binary, started inside a bubblewrap sandbox
// with SandboxBridgeArg, as the bridge to the egress proxy (ENV-010, OQ-037).
// The sandbox's private network cannot reach this computer's loopback, where the
// proxy listens, so the proxy's Unix socket is mounted in, and the bridge listens
// on the sandbox's own loopback at the port the proxy variables name. It returns
// an exit status; on success the first form does not return at all.
//
//	sandbox-bridge -listen 127.0.0.1:PORT -socket PATH -- ARGV...
//
// binds the port, leaves a server for it behind and becomes ARGV, so the command
// keeps its process, exit status and signals as without a bridge. The server is
// started through a second process that exits at once (-detach), so the sandbox's
// first process, not the command, is the server's parent: a command waiting for
// all of its children must not wait for it. It ends with the sandbox, whose
// process namespace goes when the command does.
func RunSandboxBridge(args []string) int {
	if err := runBridge(args); err != nil {
		fmt.Fprintf(os.Stderr, "freeloop: the sandbox's way to the egress proxy failed: %v\n", err)
		return 125
	}
	return 0
}

func runBridge(args []string) error {
	var listen, socket, mode string
	var argv []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--":
			argv = args[i+1:]
			i = len(args)
		case (a == "-listen" || a == "-socket") && i+1 < len(args):
			if a == "-listen" {
				listen = args[i+1]
			} else {
				socket = args[i+1]
			}
			i++
		case a == "-detach" || a == "-serve":
			mode = a
		default:
			return fmt.Errorf("unknown argument %q", a)
		}
	}
	if socket == "" {
		return errors.New("no socket")
	}
	switch mode {
	case "-serve":
		ln, err := net.FileListener(os.NewFile(3, "listener"))
		if err != nil {
			return err
		}
		serveBridge(ln, socket)
		return nil
	case "-detach":
		return startBridge("-serve", socket, os.NewFile(3, "listener"))
	}
	if listen == "" || len(argv) == 0 {
		return errors.New("needs -listen and a command")
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return err
	}
	f, err := ln.(*net.TCPListener).File()
	if err != nil {
		return err
	}
	// The port is bound from here on: a connection waits in its queue until the
	// server takes it, so the command needs no waiting for the server.
	if err := startBridge("-detach", socket, f); err != nil {
		return err
	}
	f.Close()
	ln.Close()
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, argv, os.Environ())
}

// startBridge runs this binary again in mode, with the listener as its fd 3 and
// nothing on its standard streams, which would otherwise keep the command's
// output open after it ended; and waits for it to exit, which -serve never does.
func startBridge(mode, socket string, listener *os.File) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer null.Close()
	p, err := os.StartProcess(self, []string{self, SandboxBridgeArg, mode, "-socket", socket}, &os.ProcAttr{Files: []*os.File{null, null, null, listener}})
	if err != nil {
		return err
	}
	if mode == "-serve" {
		p.Release()
		return nil
	}
	st, err := p.Wait()
	if err != nil {
		return err
	}
	if !st.Success() {
		return fmt.Errorf("the bridge did not start: %s", st)
	}
	return nil
}

// serveBridge forwards each connection on ln to the socket, both ways.
func serveBridge(ln net.Listener, socket string) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer c.Close()
			u, err := net.Dial("unix", socket)
			if err != nil {
				return
			}
			defer u.Close()
			done := make(chan struct{}, 2)
			pipe := func(dst, src net.Conn) {
				io.Copy(dst, src)
				if cw, ok := dst.(interface{ CloseWrite() error }); ok {
					cw.CloseWrite()
				}
				done <- struct{}{}
			}
			go pipe(u, c)
			go pipe(c, u)
			<-done
			<-done
		}()
	}
}
