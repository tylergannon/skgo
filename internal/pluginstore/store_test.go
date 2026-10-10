package pluginstore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHintsIgnoreEmptyHostDirectories(t *testing.T) {
	home := t.TempDir()
	empty := Dir(home, "failed-install-host")
	if err := os.MkdirAll(empty, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(empty, ".install.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	hints, err := Hints(home, "current")
	if err != nil || len(hints) != 0 {
		t.Fatalf("empty host reported as retained install: %v, %v", hints, err)
	}
	retained := Slot(home, "old-host", "example.com/plugin")
	if err := os.MkdirAll(filepath.Dir(retained), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(retained, []byte("opaque old binary; must not be loaded"), 0600); err != nil {
		t.Fatal(err)
	}
	hints, err = Hints(home, "current")
	if err != nil || len(hints) != 2 || !strings.Contains(hints[0], "1 retained installation") || !strings.Contains(hints[1], "missing/malformed") {
		t.Fatalf("retained host missing: %v, %v", hints, err)
	}
	if strings.Contains(strings.Join(hints, "\n"), "failed-install-host") {
		t.Fatal("empty host still named")
	}
}
