package app

import (
	"os"
	"path/filepath"
	"testing"

	"mygoify/internal/secret"
)

// The login cmd/poc kept moves into the secret store, and never over a
// login the store keeps already.
func TestAdoptPOCLogin(t *testing.T) {
	dir := t.TempDir()
	keep := secret.Files(filepath.Join(dir, "secrets"))
	file := filepath.Join(dir, "poc-token.json")
	os.WriteFile(file, []byte("poc"), 0o600)
	adoptPOCLogin(keep, file)
	if b, _ := keep.Get(secret.SpotifyLogin); string(b) != "poc" {
		t.Fatalf("store keeps %q, want the POC's login", b)
	}
	if _, err := os.Stat(file); err == nil {
		t.Error("the POC's file is left behind")
	}

	keep.Set(secret.SpotifyLogin, []byte("app"))
	os.WriteFile(file, []byte("poc again"), 0o600)
	adoptPOCLogin(keep, file)
	if b, _ := keep.Get(secret.SpotifyLogin); string(b) != "app" {
		t.Errorf("store keeps %q, want the login it had", b)
	}
}
