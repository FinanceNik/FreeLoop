//go:build linux

package linux

import (
	"log"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
)

// Sessions from the phone (GUI-052) need the app running while the user is away.

// NoNap does nothing: Linux desktops do not slow a minimised app the way App Nap
// does.
func NoNap(string) (release func()) { return func() {} }

// KeepAwake asks logind not to let the computer sleep when idle until release is
// called, through an inhibitor lock that ends when its descriptor closes. A closed
// lid is logind's own setting and may still suspend.
func KeepAwake(reason string) (release func()) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		log.Printf("linux: keep awake: %v", err)
		return func() {}
	}
	var fd dbus.UnixFD
	err = conn.Object("org.freedesktop.login1", "/org/freedesktop/login1").
		Call("org.freedesktop.login1.Manager.Inhibit", 0, "sleep:idle", "Freeloop", reason, "block").Store(&fd)
	if err != nil {
		conn.Close()
		log.Printf("linux: keep awake: %v", err)
		return func() {}
	}
	lock := os.NewFile(uintptr(fd), "logind-inhibit")
	var once sync.Once
	return func() {
		once.Do(func() {
			lock.Close()
			conn.Close()
		})
	}
}
