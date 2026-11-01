package secret

import (
	"errors"
	"os"
	"testing"
)

// The Secret Service keeps, replaces and forgets a secret. It touches the
// listener's own keychain, so it runs only with MYGOIFY_KEYCHAIN set.
func TestSecretService(t *testing.T) {
	if os.Getenv("MYGOIFY_KEYCHAIN") == "" {
		t.Skip("set MYGOIFY_KEYCHAIN to try the session's Secret Service")
	}
	s, name := SecretService{}, "test-secret"
	defer s.Delete(name)
	for _, v := range []string{"first", "second"} {
		if err := s.Set(name, []byte(v)); err != nil {
			t.Fatal(err)
		}
		if got, err := s.Get(name); err != nil || string(got) != v {
			t.Fatalf("Get = %q, %v; want %q", got, err, v)
		}
	}
	if err := s.Delete(name); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(name); !errors.Is(err, ErrNotFound) {
		t.Errorf("after Delete, err %v; want ErrNotFound", err)
	}
}

// A secret kept in a file moves into the Secret Service when it is first
// asked for, and the file goes.
func TestSecretServiceTakesOverFiles(t *testing.T) {
	if os.Getenv("MYGOIFY_KEYCHAIN") == "" {
		t.Skip("set MYGOIFY_KEYCHAIN to try the session's Secret Service")
	}
	dir, name := Files(t.TempDir()), "test-secret"
	defer SecretService{}.Delete(name)
	if err := dir.Set(name, []byte("kept")); err != nil {
		t.Fatal(err)
	}
	if got, err := Default(string(dir)).Get(name); err != nil || string(got) != "kept" {
		t.Fatalf("Get = %q, %v; want kept", got, err)
	}
	if _, err := dir.Get(name); !errors.Is(err, ErrNotFound) {
		t.Errorf("the file stayed: %v", err)
	}
	if got, err := (SecretService{}).Get(name); err != nil || string(got) != "kept" {
		t.Errorf("Secret Service has %q, %v; want kept", got, err)
	}
}
