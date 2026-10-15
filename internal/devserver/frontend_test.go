package devserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFrontendReadinessFollowsSameProcessRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ready.json")
	f := &frontend{readyFile: path, ready: frontendReady{Socket: "/tmp/fixture/vite.sock", Instance: "first"}}
	write := func(raw string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(`{"socket":"/tmp/fixture/vite.sock","instance":"first"}`)
	if changed, err := f.refresh(); err != nil || changed {
		t.Fatalf("stable readiness: changed=%t err=%v", changed, err)
	}
	write(`{"socket":"/tmp/fixture/vite.sock","instance":"second","tooling":["/tool/"]}`)
	if changed, err := f.refresh(); err != nil || !changed || f.ready.Instance != "second" || len(f.ready.Tooling) != 1 {
		t.Fatalf("same-port restart not adopted: %+v changed=%t err=%v", f.ready, changed, err)
	}
	write(`{"socket":"/tmp/fixture/vite.sock","instance":"third"}`)
	if changed, err := f.refresh(); err != nil || !changed || f.ready.Socket != "/tmp/fixture/vite.sock" {
		t.Fatalf("new private port not adopted: %+v changed=%t err=%v", f.ready, changed, err)
	}
	write(`{"socket":"/tmp/other/vite.sock","instance":"bad"}`)
	if _, err := f.refresh(); err == nil || f.ready.Instance != "third" {
		t.Fatalf("invalid readiness replaced working target: %+v err=%v", f.ready, err)
	}
}
