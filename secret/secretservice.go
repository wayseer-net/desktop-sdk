//go:build !darwin && !windows

package secret

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	ssName    = "org.freedesktop.secrets"
	ssPath    = dbus.ObjectPath("/org/freedesktop/secrets")
	ssService = "org.freedesktop.Secret.Service"
	noPrompt  = dbus.ObjectPath("/")
	// unlockWait is how long the owner has to answer the keyring's unlock prompt.
	unlockWait = 2 * time.Minute
)

// Freedesktop reads items whose attributes are service and account, as secret-tool stores
// them, from the freedesktop Secret Service.
type Freedesktop struct {
	Address string // the bus; empty for the session bus
}

// wireSecret is the Secret Service's Secret struct.
type wireSecret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

// Get reads the first item matching service and account, unlocking it if need be.
func (s Freedesktop) Get(service, account string) (string, error) {
	conn, err := s.connect()
	if err != nil {
		return "", NoKeyring("no session bus to reach the Secret Service on")
	}
	defer func() { _ = conn.Close() }()
	items, err := find(conn, map[string]string{"service": service, "account": account})
	if err != nil {
		return "", err
	}
	return read(conn, items[0])
}

func (s Freedesktop) connect() (*dbus.Conn, error) {
	addr := s.Address
	if addr == "" {
		addr = sessionBusAddress()
	}
	if addr == "" {
		return nil, errors.New("no session bus")
	}
	return dbus.Connect(addr)
}

// sessionBusAddress finds the running session bus; unlike godbus, it never launches one.
func sessionBusAddress() string {
	if a := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); a != "" {
		return a
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		p := filepath.Join(dir, "bus")
		if fi, err := os.Stat(p); err == nil && fi.Mode()&os.ModeSocket != 0 {
			return "unix:path=" + p
		}
	}
	return ""
}

// find returns the unlocked items matching attrs, unlocking locked ones.
func find(conn *dbus.Conn, attrs map[string]string) ([]dbus.ObjectPath, error) {
	var unlocked, locked []dbus.ObjectPath
	if err := conn.Object(ssName, ssPath).Call(ssService+".SearchItems", 0, attrs).Store(&unlocked, &locked); err != nil {
		return nil, busError(err)
	}
	if len(unlocked) == 0 && len(locked) > 0 {
		var err error
		if unlocked, err = unlock(conn, locked); err != nil {
			return nil, err
		}
	}
	if len(unlocked) == 0 {
		return nil, ErrNotFound
	}
	return unlocked, nil
}

// unlock asks the service to unlock items, waiting for the owner if it prompts them.
func unlock(conn *dbus.Conn, items []dbus.ObjectPath) ([]dbus.ObjectPath, error) {
	var unlocked []dbus.ObjectPath
	var prompt dbus.ObjectPath
	if err := conn.Object(ssName, ssPath).Call(ssService+".Unlock", 0, items).Store(&unlocked, &prompt); err != nil {
		return nil, busError(err)
	}
	if prompt == noPrompt {
		return unlocked, nil
	}
	return awaitPrompt(conn, prompt)
}

// awaitPrompt shows the service's prompt and waits for its Completed signal.
func awaitPrompt(conn *dbus.Conn, prompt dbus.ObjectPath) ([]dbus.ObjectPath, error) {
	match := []dbus.MatchOption{dbus.WithMatchObjectPath(prompt), dbus.WithMatchInterface("org.freedesktop.Secret.Prompt"), dbus.WithMatchMember("Completed")}
	if err := conn.AddMatchSignal(match...); err != nil {
		return nil, busError(err)
	}
	signals := make(chan *dbus.Signal, 1)
	conn.Signal(signals)
	defer conn.RemoveSignal(signals)
	if err := conn.Object(ssName, prompt).Call("org.freedesktop.Secret.Prompt.Prompt", 0, "").Err; err != nil {
		return nil, busError(err)
	}
	timeout := time.After(unlockWait)
	for {
		select {
		case sig := <-signals:
			if sig.Path == prompt && len(sig.Body) == 2 {
				return completed(sig.Body)
			}
		case <-timeout:
			return nil, errors.New("the keyring was not unlocked in time")
		}
	}
}

// completed reads a Completed signal's dismissed flag and the unlocked items.
func completed(body []any) ([]dbus.ObjectPath, error) {
	if dismissed, _ := body[0].(bool); dismissed {
		return nil, errors.New("the keyring is locked, and unlocking it was dismissed")
	}
	v, _ := body[1].(dbus.Variant)
	paths, _ := v.Value().([]dbus.ObjectPath)
	return paths, nil
}

// read fetches an item's secret through a plain session, which the bus keeps to this user.
func read(conn *dbus.Conn, item dbus.ObjectPath) (string, error) {
	var out dbus.Variant
	var session dbus.ObjectPath
	if err := conn.Object(ssName, ssPath).Call(ssService+".OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&out, &session); err != nil {
		return "", busError(err)
	}
	defer conn.Object(ssName, session).Call("org.freedesktop.Secret.Session.Close", 0)
	var sec wireSecret
	if err := conn.Object(ssName, item).Call("org.freedesktop.Secret.Item.GetSecret", 0, session).Store(&sec); err != nil {
		return "", busError(err)
	}
	return string(sec.Value), nil
}

// busError turns the bus's word that nothing owns the Secret Service name into ErrNoKeyring.
// D-Bus errors carry names and paths, never the secret.
func busError(err error) error {
	var e dbus.Error
	var ep *dbus.Error
	name := ""
	switch {
	case errors.As(err, &e):
		name = e.Name
	case errors.As(err, &ep):
		name = ep.Name
	}
	switch name {
	case "org.freedesktop.DBus.Error.ServiceUnknown", "org.freedesktop.DBus.Error.NameHasNoOwner":
		return NoKeyring("the Secret Service is not running")
	case "":
		return fmt.Errorf("the Secret Service: %w", err)
	}
	return fmt.Errorf("the Secret Service: %s", name)
}
