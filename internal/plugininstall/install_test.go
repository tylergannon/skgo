package plugininstall

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/pluginstore"
)

func TestPublishFailureKeepsPriorSelection(t *testing.T) {
	dir := t.TempDir()
	slot := filepath.Join(dir, "module.so")
	ref := pluginstore.Reference(slot)
	os.WriteFile(slot, []byte("working prior binary"), 0755)
	os.WriteFile(ref, []byte("prior source"), 0644)
	if err := publish(filepath.Join(dir, "missing.so"), slot, pluginstore.Source{Module: "example.com/new", Version: "v1.0.0"}); err == nil {
		t.Fatal("missing candidate accepted")
	}
	for name, want := range map[string]string{slot: "working prior binary", ref: "prior source"} {
		if b, e := os.ReadFile(name); e != nil || string(b) != want {
			t.Fatalf("%s changed: %q %v", name, b, e)
		}
	}
	candidate := filepath.Join(dir, "candidate.so")
	os.WriteFile(candidate, []byte("new binary"), 0755)
	if err := publish(candidate, slot, pluginstore.Source{Module: "example.com/new", Version: "v1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(slot); string(b) != "new binary" {
		t.Fatalf("candidate not published: %q", b)
	}
	if b, _ := os.ReadFile(ref); string(b) != `{"module":"example.com/new","version":"v1.0.0"}` {
		t.Fatalf("source reference: %q", b)
	}
}
func TestHostLockSerializesAndCancellationReleasesWait(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".install.lock")
	unlock, err := lock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	second, err := lock(ctx, path)
	if err == nil {
		second()
		unlock()
		t.Fatal("two publishers held the same host lock")
	}
	unlock()
	third, err := lock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	third()
}
