package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Sessions from the phone (GUI-052): the app answers on a port of this computer,
// which the user makes reachable from their phone over their own private network
// (Tailscale). Only paired devices get in.

// DefaultRemotePort is where the phone server listens unless set otherwise.
const DefaultRemotePort = 34217

// Remote is the phone server's settings.
type Remote struct {
	// Enabled turns it on. Set in a file, it waits for the user's approval.
	Enabled bool `yaml:"enabled,omitempty" json:"enabled"`
	// Port is the port on 127.0.0.1 it listens on; 0 means DefaultRemotePort.
	Port int `yaml:"port,omitempty" json:"port"`
	// Host is the name the phone reaches it by, such as the computer's Tailscale
	// name; requests naming another host are refused.
	Host string `yaml:"host,omitempty" json:"host"`
	// KeepAwake keeps the computer from sleeping while a session runs or waits.
	KeepAwake bool `yaml:"keepAwake,omitempty" json:"keepAwake"`
}

// RemotePort is the port to listen on.
func (r Remote) RemotePort() int {
	if r.Port > 0 && r.Port < 65536 {
		return r.Port
	}
	return DefaultRemotePort
}

// RemoteFingerprint identifies remote.enabled, which a file turns on only with the
// user's approval.
func RemoteFingerprint() string {
	return commandFingerprint("setting", "remote.enabled")
}

// DevicesFile lists the paired phones.
const DevicesFile = "devices.json"

// Device is a paired phone. Its token is kept only as a hash.
type Device struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	TokenHash string `json:"tokenHash"`
	// Login is the Tailscale user it paired as, when the request said; another
	// login's requests are refused with its token.
	Login    string    `json:"login,omitempty"`
	Paired   time.Time `json:"paired"`
	LastSeen time.Time `json:"lastSeen"`
	// Push is the phone's Web Push subscription, once it turned notifications on.
	Push *PushSubscription `json:"push,omitempty"`
}

// PushSubscription is what a browser's push service gave the phone app: where to
// send, and the keys that encrypt what is sent (RFC 8291). One per phone.
type PushSubscription struct {
	Endpoint string `json:"endpoint"`
	P256dh   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

// LoadDevices reads the paired phones.
func LoadDevices(dir string) ([]Device, error) {
	b, err := os.ReadFile(filepath.Join(dir, DevicesFile))
	if errors.Is(err, os.ErrNotExist) {
		return []Device{}, nil
	}
	if err != nil {
		return nil, err
	}
	var d []Device
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("%s: %w", DevicesFile, err)
	}
	return d, nil
}

// SaveDevices replaces the paired phones; only the user can read the file.
func SaveDevices(dir string, d []Device) error {
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, DevicesFile), b)
}
