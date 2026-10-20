//go:build windows

package desktop

import (
	"net/http/httptest"
	"opsbuddy/internal/app"
	"testing"
)

func TestDesktopTransportIsScoped(t *testing.T) {
	core, err := app.New(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	c := New("")
	c.core = core
	c.handler = core.Handler()
	for _, tc := range []struct {
		host, origin, path string
		status             int
	}{
		{"wails.localhost", "http://wails.localhost", "/api/state", 200},
		{"wails.localhost", "https://untrusted.example", "/api/state", 403},
		{"127.0.0.1", "", "/api/state", 403},
		{"wails.localhost", "", "/not-api", 404},
	} {
		r := httptest.NewRequest("GET", "http://"+tc.host+tc.path, nil)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		c.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%+v: got %d %s", tc, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest("GET", "http://wails.localhost/api/state", nil)
	w := httptest.NewRecorder()
	core.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("desktop transport widened browser server origins")
	}
}
