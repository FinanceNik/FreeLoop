//go:build linux

package linux

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/FinanceNik/Freeloop/internal/secrets"
)

// The Secret Service (https://specifications.freedesktop.org/secret-service-spec/) is the
// Linux desktop's keyring: GNOME Keyring and KWallet both serve it on the session bus.
// SecretService stores each secret as an item of the default collection, with the
// attributes service and account that name the reference, as the macOS Keychain
// does with a generic password's service and account. Nothing is ever kept in a file
// of the app's own: with no Secret Service, the store says so.

const (
	ssName       = "org.freedesktop.secrets"
	ssPath       = "/org/freedesktop/secrets"
	ifService    = "org.freedesktop.Secret.Service"
	ifCollection = "org.freedesktop.Secret.Collection"
	ifItem       = "org.freedesktop.Secret.Item"
	ifPrompt     = "org.freedesktop.Secret.Prompt"
	ifSession    = "org.freedesktop.Secret.Session"
	noObject     = dbus.ObjectPath("/")

	// schema is the attribute libsecret's generic schema sets, so `secret-tool lookup
	// service S account A` finds what the app stored.
	schemaAttr  = "xdg:schema"
	schemaValue = "org.freedesktop.Secret.Generic"
)

// ErrNoSecretService is returned when no keyring answers on the session bus.
var ErrNoSecretService = errors.New("no Secret Service: install and unlock GNOME Keyring or KWallet")

// SecretService is a secrets.Store over the freedesktop Secret Service.
type SecretService struct {
	// PromptTimeout is how long an unlock prompt waits for the user; 2 minutes if zero.
	PromptTimeout time.Duration
	// Address is the bus to connect to; the session bus if empty.
	Address string
}

var (
	_ secrets.Store  = (*SecretService)(nil)
	_ secrets.Lister = (*SecretService)(nil)
)

// NewSecretService returns the store over the session bus.
func NewSecretService() *SecretService { return &SecretService{} }

// secret is the Secret Service's (oayays) struct.
type secret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

// conn is one connection with an open session.
type conn struct {
	c       *dbus.Conn
	svc     dbus.BusObject
	session dbus.ObjectPath
	timeout time.Duration
}

// busAddress is the session bus's address. godbus itself would start dbus-launch where
// there is none; the app never starts a bus: no bus means no keyring.
func busAddress() string {
	if a := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); a != "" {
		return a
	}
	// systemd's per-user bus, which a desktop session does not always export.
	p := "/run/user/" + strconv.Itoa(os.Getuid()) + "/bus"
	if _, err := os.Stat(p); err == nil {
		return "unix:path=" + p
	}
	return ""
}

// noService says whether err means that nothing serves the Secret Service.
func noService(err error) bool {
	var de dbus.Error
	if errors.As(err, &de) {
		switch de.Name {
		case "org.freedesktop.DBus.Error.ServiceUnknown", "org.freedesktop.DBus.Error.NameHasNoOwner",
			"org.freedesktop.DBus.Error.NoReply", "org.freedesktop.DBus.Error.Spawn.ServiceNotFound",
			"org.freedesktop.DBus.Error.Spawn.ChildExited", "org.freedesktop.DBus.Error.Spawn.Failed":
			return true
		}
	}
	return false
}

func missing(err error) error {
	return fmt.Errorf("secrets: %w (%v)", ErrNoSecretService, err)
}

func (s *SecretService) open() (*conn, error) {
	addr := s.Address
	if addr == "" {
		addr = busAddress()
	}
	if addr == "" {
		return nil, missing(errors.New("no session bus"))
	}
	c, err := dbus.Connect(addr)
	if err != nil {
		return nil, missing(err)
	}
	o := &conn{c: c, svc: c.Object(ssName, ssPath), timeout: s.PromptTimeout}
	if o.timeout == 0 {
		o.timeout = 2 * time.Minute
	}
	// A plain session: the secret crosses the user's own session bus unencrypted, which
	// only processes of that user can reach, as libsecret does without its encrypted one.
	var out dbus.Variant
	if err := o.svc.Call(ifService+".OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&out, &o.session); err != nil {
		c.Close()
		if noService(err) {
			return nil, missing(err)
		}
		return nil, fmt.Errorf("secrets: open a session: %w", err)
	}
	return o, nil
}

func (o *conn) close() {
	if o.session != "" {
		o.c.Object(ssName, o.session).Call(ifSession+".Close", 0)
	}
	o.c.Close()
}

// prompt runs a prompt the service asks for and returns what it came to.
func (o *conn) prompt(p dbus.ObjectPath) (dbus.Variant, error) {
	ch := make(chan *dbus.Signal, 16)
	o.c.Signal(ch)
	defer o.c.RemoveSignal(ch)
	opts := []dbus.MatchOption{dbus.WithMatchObjectPath(p), dbus.WithMatchInterface(ifPrompt), dbus.WithMatchMember("Completed")}
	if err := o.c.AddMatchSignal(opts...); err != nil {
		return dbus.Variant{}, err
	}
	defer o.c.RemoveMatchSignal(opts...)
	if err := o.c.Object(ssName, p).Call(ifPrompt+".Prompt", 0, "").Err; err != nil {
		return dbus.Variant{}, fmt.Errorf("secrets: show the keyring prompt: %w", err)
	}
	timer := time.NewTimer(o.timeout)
	defer timer.Stop()
	for {
		select {
		case sig := <-ch:
			if sig == nil {
				return dbus.Variant{}, errors.New("secrets: the connection to the keyring closed")
			}
			if sig.Path != p || sig.Name != ifPrompt+".Completed" || len(sig.Body) != 2 {
				continue
			}
			dismissed, _ := sig.Body[0].(bool)
			result, _ := sig.Body[1].(dbus.Variant)
			if dismissed {
				return dbus.Variant{}, errors.New("secrets: the keyring prompt was dismissed")
			}
			return result, nil
		case <-timer.C:
			o.c.Object(ssName, p).Call(ifPrompt+".Dismiss", 0)
			return dbus.Variant{}, errors.New("secrets: the keyring prompt timed out")
		}
	}
}

// collection returns the default collection, creating one when there is none.
func (o *conn) collection() (dbus.ObjectPath, error) {
	var path dbus.ObjectPath
	if err := o.svc.Call(ifService+".ReadAlias", 0, "default").Store(&path); err != nil {
		return "", fmt.Errorf("secrets: find the default keyring: %w", err)
	}
	if path != noObject {
		return path, nil
	}
	props := map[string]dbus.Variant{ifCollection + ".Label": dbus.MakeVariant("Login")}
	var coll, prompt dbus.ObjectPath
	if err := o.svc.Call(ifService+".CreateCollection", 0, props, "default").Store(&coll, &prompt); err != nil {
		return "", fmt.Errorf("secrets: there is no default keyring and none could be created: %w", err)
	}
	if prompt != noObject {
		res, err := o.prompt(prompt)
		if err != nil {
			return "", err
		}
		coll, _ = res.Value().(dbus.ObjectPath)
	}
	if coll == "" || coll == noObject {
		return "", errors.New("secrets: there is no default keyring")
	}
	return coll, nil
}

// unlocked makes sure collection c is unlocked, following the service's prompt.
func (o *conn) unlocked(c dbus.ObjectPath) error {
	locked, err := o.c.Object(ssName, c).GetProperty(ifCollection + ".Locked")
	if err != nil {
		return fmt.Errorf("secrets: read the keyring's state: %w", err)
	}
	if l, _ := locked.Value().(bool); !l {
		return nil
	}
	var done []dbus.ObjectPath
	var prompt dbus.ObjectPath
	if err := o.svc.Call(ifService+".Unlock", 0, []dbus.ObjectPath{c}).Store(&done, &prompt); err != nil {
		return fmt.Errorf("secrets: unlock the keyring: %w", err)
	}
	if prompt != noObject {
		if _, err := o.prompt(prompt); err != nil {
			return fmt.Errorf("secrets: the keyring is locked; unlock it and try again (%w)", err)
		}
	}
	return nil
}

func attributes(ref secrets.Ref) map[string]string {
	return map[string]string{"service": ref.Service, "account": ref.Account}
}

// find returns the items of collection c that have all of attrs.
func (o *conn) find(c dbus.ObjectPath, attrs map[string]string) ([]dbus.ObjectPath, error) {
	var items []dbus.ObjectPath
	if err := o.c.Object(ssName, c).Call(ifCollection+".SearchItems", 0, attrs).Store(&items); err != nil {
		return nil, fmt.Errorf("secrets: search the keyring: %w", err)
	}
	return items, nil
}

// with runs f on the default collection over a fresh connection.
func (s *SecretService) with(f func(o *conn, coll dbus.ObjectPath) error) error {
	o, err := s.open()
	if err != nil {
		return err
	}
	defer o.close()
	coll, err := o.collection()
	if err != nil {
		return err
	}
	return f(o, coll)
}

func (s *SecretService) Get(ref secrets.Ref) (string, error) {
	var value string
	err := s.with(func(o *conn, coll dbus.ObjectPath) error {
		items, err := o.find(coll, attributes(ref))
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return secrets.ErrNotFound
		}
		if err := o.unlocked(coll); err != nil {
			return err
		}
		var sec secret
		if err := o.c.Object(ssName, items[0]).Call(ifItem+".GetSecret", 0, o.session).Store(&sec); err != nil {
			return fmt.Errorf("secrets: read %s: %w", ref, err)
		}
		value = string(sec.Value)
		return nil
	})
	return value, err
}

func (s *SecretService) Set(ref secrets.Ref, value string) error {
	return s.with(func(o *conn, coll dbus.ObjectPath) error {
		if err := o.unlocked(coll); err != nil {
			return err
		}
		attrs := attributes(ref)
		attrs[schemaAttr] = schemaValue
		props := map[string]dbus.Variant{
			ifItem + ".Label":      dbus.MakeVariant("Freeloop: " + ref.Service + "/" + ref.Account),
			ifItem + ".Attributes": dbus.MakeVariant(attrs),
		}
		sec := secret{Session: o.session, Parameters: []byte{}, Value: []byte(value), ContentType: "text/plain; charset=utf8"}
		var item, prompt dbus.ObjectPath
		if err := o.c.Object(ssName, coll).Call(ifCollection+".CreateItem", 0, props, sec, true).Store(&item, &prompt); err != nil {
			return fmt.Errorf("secrets: store %s: %w", ref, err)
		}
		if prompt != noObject {
			if _, err := o.prompt(prompt); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *SecretService) Delete(ref secrets.Ref) error {
	return s.with(func(o *conn, coll dbus.ObjectPath) error {
		items, err := o.find(coll, attributes(ref))
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return secrets.ErrNotFound
		}
		if err := o.unlocked(coll); err != nil {
			return err
		}
		for _, item := range items {
			var prompt dbus.ObjectPath
			if err := o.c.Object(ssName, item).Call(ifItem+".Delete", 0).Store(&prompt); err != nil {
				return fmt.Errorf("secrets: delete %s: %w", ref, err)
			}
			if prompt != noObject {
				if _, err := o.prompt(prompt); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Accounts lists the accounts held under a service, reading no secret (secrets.Lister).
func (s *SecretService) Accounts(service string) ([]string, error) {
	var out []string
	err := s.with(func(o *conn, coll dbus.ObjectPath) error {
		items, err := o.find(coll, map[string]string{"service": service})
		if err != nil {
			return err
		}
		for _, item := range items {
			v, err := o.c.Object(ssName, item).GetProperty(ifItem + ".Attributes")
			if err != nil {
				return fmt.Errorf("secrets: list %s: %w", service, err)
			}
			if attrs, ok := v.Value().(map[string]string); ok && attrs["account"] != "" {
				out = append(out, attrs["account"])
			}
		}
		return nil
	})
	return out, err
}
