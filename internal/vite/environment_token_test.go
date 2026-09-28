package vite

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPrivateModuleTransportUsesFilesystemToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Skgo-Dev-Token") != "fixture-private-token" {
			http.Error(w, "Forbidden", 403)
			return
		}
		if r.URL.Path != "/__skgo_dev/module" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"export const privateValue = 123;"}`))
	}))
	defer server.Close()
	dev := NewDev(server.URL, "fixture-private-token")
	module, err := dev.Module("private/server.js", "")
	if err != nil {
		t.Fatal(err)
	}
	if module.Code != "export const privateValue = 123;" {
		t.Fatalf("module: %#v", module)
	}
	if _, err := NewDev(server.URL).Module("private/server.js", ""); err == nil {
		t.Fatal("unauthenticated private module unexpectedly loaded")
	}
}
