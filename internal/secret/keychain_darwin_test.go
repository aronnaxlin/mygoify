package secret

import (
	"errors"
	"os"
	"testing"
)

// A secret set in the keychain reads back, replaced as it is set again,
// and is gone once deleted. It writes to the login keychain, so runs
// only with MYGOIFY_KEYCHAIN set.
func TestKeychain(t *testing.T) {
	if os.Getenv("MYGOIFY_KEYCHAIN") == "" {
		t.Skip("set MYGOIFY_KEYCHAIN to try the login keychain")
	}
	k, name := Keychain{}, "test-secret"
	defer k.Delete(name)
	for _, v := range []string{"first", "second"} {
		if err := k.Set(name, []byte(v)); err != nil {
			t.Fatal(err)
		}
		if got, err := k.Get(name); err != nil || string(got) != v {
			t.Fatalf("Get = %q, %v; want %q", got, err, v)
		}
	}
	if err := k.Delete(name); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Get(name); !errors.Is(err, ErrNotFound) {
		t.Errorf("after Delete, err %v; want ErrNotFound", err)
	}
}
