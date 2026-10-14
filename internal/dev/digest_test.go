package dev

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A request walking the tree must not hold up another request's freshness
// check. Keep one scan open while another observes an intervening edit.
func TestDigestScansOverlapAndValidateTheirOwnFileVersion(t *testing.T) {
	file := filepath.Join(t.TempDir(), "handler.go")
	first := []byte("package fixture\nconst Value = 1\n")
	second := []byte("package fixture\nconst Value = 2\n")
	if err := os.WriteFile(file, first, 0644); err != nil {
		t.Fatal(err)
	}
	original, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	cache := &fileDigests{}
	old := cache.scan()
	if got, err := old.sum(file); err != nil || got != fmt.Sprintf("%x", sha256.Sum256(first)) {
		old.close()
		t.Fatalf("first digest %q: %v", got, err)
	}
	if err := os.WriteFile(file, second, 0644); err != nil {
		old.close()
		t.Fatal(err)
	}
	if err := os.Chtimes(file, original.ModTime(), original.ModTime()); err != nil {
		old.close()
		t.Fatal(err)
	}
	type result struct {
		sum string
		err error
	}
	done := make(chan result, 1)
	go func() {
		current := cache.scan()
		sum, err := current.sum(file)
		current.close()
		done <- result{sum, err}
	}()
	select {
	case r := <-done:
		if r.err != nil || r.sum != fmt.Sprintf("%x", sha256.Sum256(second)) {
			old.close()
			t.Fatalf("concurrent edit digest %q: %v", r.sum, r.err)
		}
	case <-time.After(2 * time.Second):
		old.close()
		<-done
		t.Fatal("second request's scan blocked behind the open first scan")
	}
	// An older scan can publish last, but its metadata cannot authorize replay
	// of stale bytes: every later scan checks the current file version itself.
	old.close()
	latest := cache.scan()
	got, err := latest.sum(file)
	latest.close()
	if err != nil || got != fmt.Sprintf("%x", sha256.Sum256(second)) {
		t.Fatalf("stale published digest %q: %v", got, err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	empty := cache.scan()
	empty.close()
	if len(cache.entries) != 0 {
		t.Fatal("cache retained a removed source")
	}
}
