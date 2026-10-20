//go:build linux

package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxCredentialRoundtripAndTamper(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	secret := "test-only-local-secret"
	encrypted, err := seal(secret)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encrypted, secret) {
		t.Fatal("plaintext stored")
	}
	plain, err := unseal(encrypted)
	if err != nil || plain != secret {
		t.Fatal("roundtrip", err)
	}
	other, _ := seal(secret)
	if other == encrypted {
		t.Fatal("nonce reused")
	}
	damaged := []byte(encrypted)
	damaged[len(damaged)-6] ^= 1
	if _, err = unseal(string(damaged)); err == nil {
		t.Fatal("tampering accepted")
	}
	if _, err = unseal("windows-dpapi-data"); err == nil {
		t.Fatal("foreign ciphertext accepted")
	}
	a := newTestApp(t)
	w := request(t, a, "POST", "/api/models", map[string]string{"name": "test", "family": "openai", "baseUrl": "https://example.com/v1", "modelId": "test", "apiKey": secret})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	state := request(t, a, "GET", "/api/state", nil).Body.String()
	if strings.Contains(state, secret) || strings.Contains(state, encrypted) {
		t.Fatal("secret exposed")
	}
}
func TestLinuxCredentialRejectsUnsafeKey(t *testing.T) {
	config := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	if _, err := seal("sample"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(config, "opsbuddy-credentials", "key")
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := seal("sample"); err == nil {
		t.Fatal("world-readable key accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(config, "other")
	if err := os.WriteFile(target, make([]byte, 32), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := seal("sample"); err == nil {
		t.Fatal("symlink key accepted")
	}
}
