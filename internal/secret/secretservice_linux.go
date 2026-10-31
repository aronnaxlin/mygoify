package secret

import (
	"errors"
	"fmt"

	"github.com/godbus/dbus/v5"
)

// SecretService keeps secrets in the Secret Service of the listener's
// session bus, as GNOME Keyring and KeePassXC offer it: each secret an
// item of the default collection, found by the app's service and its
// name.
type SecretService struct{}

// service is the attribute that tells the app's items from others'.
const service = "io.github.aronnaxlin.mygoify"

const (
	ssName       = "org.freedesktop.secrets"
	ssPath       = dbus.ObjectPath("/org/freedesktop/secrets")
	ssDefault    = dbus.ObjectPath("/org/freedesktop/secrets/aliases/default")
	ssService    = "org.freedesktop.Secret.Service"
	ssCollection = "org.freedesktop.Secret.Collection"
	ssItem       = "org.freedesktop.Secret.Item"
	ssPrompt     = "org.freedesktop.Secret.Prompt"
)

// ssSecret is how the Secret Service passes a secret.
type ssSecret struct {
	Session     dbus.ObjectPath
	Params      []byte
	Value       []byte
	ContentType string
}

// ssConn is a connection to the Secret Service with a session open, in
// which secrets pass as they are: the session bus is the listener's.
type ssConn struct {
	bus     *dbus.Conn
	session dbus.ObjectPath
}

func openSS() (*ssConn, error) {
	bus, err := dbus.SessionBus()
	if err != nil {
		return nil, err
	}
	var out dbus.Variant
	var session dbus.ObjectPath
	if err := bus.Object(ssName, ssPath).Call(ssService+".OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&out, &session); err != nil {
		return nil, fmt.Errorf("secret: no Secret Service: %w", err)
	}
	return &ssConn{bus, session}, nil
}

func (c *ssConn) close() {
	c.bus.Object(ssName, c.session).Call("org.freedesktop.Secret.Session.Close", 0)
}

func attrs(name string) map[string]string {
	return map[string]string{"service": service, "account": name}
}

// prompt shows the prompt the Secret Service asks for, as to unlock the
// keychain, and waits for the listener to answer it.
func (c *ssConn) prompt(p dbus.ObjectPath) error {
	if p == "/" {
		return nil
	}
	rule := []dbus.MatchOption{dbus.WithMatchObjectPath(p), dbus.WithMatchInterface(ssPrompt), dbus.WithMatchMember("Completed")}
	if err := c.bus.AddMatchSignal(rule...); err != nil {
		return err
	}
	defer c.bus.RemoveMatchSignal(rule...)
	signals := make(chan *dbus.Signal, 1)
	c.bus.Signal(signals)
	defer c.bus.RemoveSignal(signals)
	if err := c.bus.Object(ssName, p).Call(ssPrompt+".Prompt", 0, "").Err; err != nil {
		return err
	}
	for s := range signals {
		if s.Path != p || s.Name != ssPrompt+".Completed" {
			continue
		}
		if dismissed, _ := s.Body[0].(bool); dismissed {
			return errors.New("secret: the keychain stayed locked")
		}
		return nil
	}
	return errors.New("secret: the session bus closed")
}

// unlock unlocks objects, asking the listener if it must.
func (c *ssConn) unlock(objects ...dbus.ObjectPath) error {
	var unlocked []dbus.ObjectPath
	var p dbus.ObjectPath
	if err := c.bus.Object(ssName, ssPath).Call(ssService+".Unlock", 0, objects).Store(&unlocked, &p); err != nil {
		return err
	}
	return c.prompt(p)
}

// find returns the item of name, unlocked, or "" without one.
func (c *ssConn) find(name string) (dbus.ObjectPath, error) {
	var unlocked, locked []dbus.ObjectPath
	if err := c.bus.Object(ssName, ssPath).Call(ssService+".SearchItems", 0, attrs(name)).Store(&unlocked, &locked); err != nil {
		return "", err
	}
	if len(unlocked) > 0 {
		return unlocked[0], nil
	}
	if len(locked) == 0 {
		return "", nil
	}
	if err := c.unlock(locked[0]); err != nil {
		return "", err
	}
	return locked[0], nil
}

func (SecretService) Get(name string) ([]byte, error) {
	c, err := openSS()
	if err != nil {
		return nil, err
	}
	defer c.close()
	item, err := c.find(name)
	if err != nil || item == "" {
		if err == nil {
			err = ErrNotFound
		}
		return nil, err
	}
	var s ssSecret
	if err := c.bus.Object(ssName, item).Call(ssItem+".GetSecret", 0, c.session).Store(&s); err != nil {
		return nil, err
	}
	return s.Value, nil
}

func (SecretService) Set(name string, v []byte) error {
	c, err := openSS()
	if err != nil {
		return err
	}
	defer c.close()
	if err := c.unlock(ssDefault); err != nil {
		return err
	}
	props := map[string]dbus.Variant{
		ssItem + ".Label":      dbus.MakeVariant("MyGoify " + name),
		ssItem + ".Attributes": dbus.MakeVariant(attrs(name)),
	}
	var item, p dbus.ObjectPath
	s := ssSecret{Session: c.session, Params: []byte{}, Value: v, ContentType: "application/octet-stream"}
	if err := c.bus.Object(ssName, ssDefault).Call(ssCollection+".CreateItem", 0, props, s, true).Store(&item, &p); err != nil {
		return err
	}
	return c.prompt(p)
}

func (SecretService) Delete(name string) error {
	c, err := openSS()
	if err != nil {
		return err
	}
	defer c.close()
	item, err := c.find(name)
	if err != nil || item == "" {
		return err
	}
	var p dbus.ObjectPath
	if err := c.bus.Object(ssName, item).Call(ssItem+".Delete", 0).Store(&p); err != nil {
		return err
	}
	return c.prompt(p)
}

// system is the Secret Service, if the session bus offers one.
func system() (Store, error) {
	c, err := openSS()
	if err != nil {
		return nil, err
	}
	c.close()
	return SecretService{}, nil
}
