//go:build !darwin && !windows

package secret_test

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"wayseer/pkg/sdk/secret"

	"github.com/godbus/dbus/v5"
)

// busConfig is a session bus that activates no services, so no real keyring can start on it.
const busConfig = `<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>session</type>
  <listen>unix:dir=%DIR%</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>`

// privateBus starts a dbus-daemon of the test's own and returns its address.
func privateBus(t *testing.T) string {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("no dbus-daemon")
	}
	dir, err := os.MkdirTemp("", "wayseer-bus") // short, as a socket path must be
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	conf := filepath.Join(dir, "bus.conf")
	if err := os.WriteFile(conf, []byte(strings.ReplaceAll(busConfig, "%DIR%", dir)), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(daemon, "--config-file="+conf, "--nofork", "--nopidfile", "--print-address=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Skipf("dbus-daemon: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Signal(os.Interrupt); _ = cmd.Wait() })
	addr, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatalf("dbus-daemon printed no address: %v", err)
	}
	return strings.TrimSpace(addr)
}

// fakeService is the part of the Secret Service API the client uses.
type fakeService struct {
	conn     *dbus.Conn
	items    map[dbus.ObjectPath]fakeItem
	prompted bool // Unlock needs a prompt rather than unlocking at once
}

type fakeItem struct {
	service, account, value string
	locked                  bool
}

type wireSecret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

const (
	fakeSession = dbus.ObjectPath("/org/freedesktop/secrets/session/1")
	fakePrompt  = dbus.ObjectPath("/org/freedesktop/secrets/prompt/1")
)

func (f *fakeService) SearchItems(attrs map[string]string) (unlocked, locked []dbus.ObjectPath, _ *dbus.Error) {
	for path, it := range f.items {
		if attrs["service"] == it.service && attrs["account"] == it.account && len(attrs) == 2 {
			if it.locked {
				locked = append(locked, path)
			} else {
				unlocked = append(unlocked, path)
			}
		}
	}
	return unlocked, locked, nil
}

func (f *fakeService) OpenSession(algorithm string, _ dbus.Variant) (dbus.Variant, dbus.ObjectPath, *dbus.Error) {
	if algorithm != "plain" {
		return dbus.MakeVariant(""), "/", dbus.NewError("org.freedesktop.DBus.Error.NotSupported", nil)
	}
	return dbus.MakeVariant(""), fakeSession, nil
}

func (f *fakeService) Unlock(paths []dbus.ObjectPath) ([]dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	if f.prompted {
		return nil, fakePrompt, nil
	}
	return f.unlock(paths), "/", nil
}

func (f *fakeService) unlock(paths []dbus.ObjectPath) []dbus.ObjectPath {
	for _, p := range paths {
		it := f.items[p]
		it.locked = false
		f.items[p] = it
	}
	return paths
}

// fakePromptObject completes at once, as though the owner typed the keyring's password.
type fakePromptObject struct{ f *fakeService }

func (p fakePromptObject) Prompt(string) *dbus.Error {
	var locked []dbus.ObjectPath
	for path, it := range p.f.items {
		if it.locked {
			locked = append(locked, path)
		}
	}
	unlocked := p.f.unlock(locked)
	_ = p.f.conn.Emit(fakePrompt, "org.freedesktop.Secret.Prompt.Completed", false, dbus.MakeVariant(unlocked))
	return nil
}

type fakeItemObject struct {
	f    *fakeService
	path dbus.ObjectPath
}

func (o fakeItemObject) GetSecret(session dbus.ObjectPath) (wireSecret, *dbus.Error) {
	it := o.f.items[o.path]
	if session != fakeSession || it.locked {
		return wireSecret{}, dbus.NewError("org.freedesktop.Secret.Error.IsLocked", nil)
	}
	return wireSecret{Session: session, Value: []byte(it.value), ContentType: "text/plain"}, nil
}

type fakeSessionObject struct{}

func (fakeSessionObject) Close() *dbus.Error { return nil }

// serve puts f on the bus at addr as org.freedesktop.secrets.
func serve(t *testing.T, addr string, f *fakeService) {
	t.Helper()
	conn, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	f.conn = conn
	export := func(v any, path dbus.ObjectPath, iface string) {
		if err := conn.Export(v, path, iface); err != nil {
			t.Fatal(err)
		}
	}
	export(f, "/org/freedesktop/secrets", "org.freedesktop.Secret.Service")
	export(fakeSessionObject{}, fakeSession, "org.freedesktop.Secret.Session")
	export(fakePromptObject{f}, fakePrompt, "org.freedesktop.Secret.Prompt")
	for path := range f.items {
		export(fakeItemObject{f, path}, path, "org.freedesktop.Secret.Item")
	}
	if reply, err := conn.RequestName("org.freedesktop.secrets", dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("own org.freedesktop.secrets: %v, %v", reply, err)
	}
}

func keyring(items ...fakeItem) *fakeService {
	f := &fakeService{items: map[dbus.ObjectPath]fakeItem{}}
	for i, it := range items {
		f.items[dbus.ObjectPath("/org/freedesktop/secrets/collection/login/"+string(rune('1'+i)))] = it
	}
	return f
}

func TestSecretServiceReadsAnItem(t *testing.T) {
	addr := privateBus(t)
	serve(t, addr, keyring(fakeItem{service: "wayseer", account: "anthropic", value: canary}, fakeItem{service: "wayseer", account: "zen", value: "other"}))
	if v, err := (secret.Freedesktop{Address: addr}).Get("wayseer", "anthropic"); err != nil || v != canary {
		t.Errorf("Get = %q, %v", v, err)
	}
	if _, err := (secret.Freedesktop{Address: addr}).Get("wayseer", "ollama"); !errors.Is(err, secret.ErrNotFound) {
		t.Errorf("an absent item: %v; want ErrNotFound", err)
	}
}

func TestSecretServiceUnlocksALockedItem(t *testing.T) {
	for _, prompted := range []bool{false, true} {
		addr := privateBus(t)
		f := keyring(fakeItem{service: "wayseer", account: "anthropic", value: canary, locked: true})
		f.prompted = prompted
		serve(t, addr, f)
		if v, err := (secret.Freedesktop{Address: addr}).Get("wayseer", "anthropic"); err != nil || v != canary {
			t.Errorf("prompted %v: Get = %q, %v", prompted, v, err)
		}
	}
}

func TestSecretServiceMissingIsNoKeyring(t *testing.T) {
	addr := privateBus(t)
	for _, a := range []string{addr, "unix:path=" + filepath.Join(t.TempDir(), "no-bus")} {
		_, err := (secret.Freedesktop{Address: a}).Get("wayseer", "anthropic")
		if !errors.Is(err, secret.ErrNoKeyring) || !strings.Contains(err.Error(), "Secret Service") {
			t.Errorf("%s: %v; want ErrNoKeyring naming the Secret Service", a, err)
		}
	}
}

func TestNoSessionBusIsNoKeyringAndStartsNone(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	_, err := (secret.Freedesktop{}).Get("wayseer", "anthropic")
	if !errors.Is(err, secret.ErrNoKeyring) || !strings.Contains(err.Error(), "no session bus") {
		t.Errorf("Get = %v; want ErrNoKeyring, no session bus", err)
	}
}
