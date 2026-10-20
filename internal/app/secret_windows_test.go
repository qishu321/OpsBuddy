//go:build windows

package app

import (
	"strings"
	"testing"
)

func TestSecretStorage(t *testing.T) {
	secret := "test-only-not-a-real-api-key"
	encrypted, e := seal(secret)
	if e != nil {
		t.Fatal(e)
	}
	if encrypted == secret || strings.Contains(encrypted, secret) {
		t.Fatal("secret stored in plaintext")
	}
	plain, e := unseal(encrypted)
	if e != nil || plain != secret {
		t.Fatal("DPAPI roundtrip failed", e)
	}
	a := newTestApp(t)
	w := request(t, a, "POST", "/api/models", map[string]string{"name": "test", "family": "openai", "baseUrl": "https://example.com/v1", "modelId": "test", "apiKey": secret})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	state := request(t, a, "GET", "/api/state", nil).Body.String()
	if strings.Contains(state, secret) || strings.Contains(state, encrypted) {
		t.Fatal("secret leaked in public state")
	}
}
