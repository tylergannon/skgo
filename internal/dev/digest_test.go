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
	old := &digestScan{}
	if got, err := old.sum(file); err != nil || got != fmt.Sprintf("%x", sha256.Sum256(first)) {
		t.Fatalf("first digest %q: %v", got, err)
	}
	if err := os.WriteFile(file, second, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file, original.ModTime(), original.ModTime()); err != nil {
		t.Fatal(err)
	}
	type result struct {
		sum string
		err error
	}
	done := make(chan result, 1)
	go func() {
		current := &digestScan{}
		sum, err := current.sum(file)
		done <- result{sum, err}
	}()
	select {
	case r := <-done:
		if r.err != nil || r.sum != fmt.Sprintf("%x", sha256.Sum256(second)) {
			t.Fatalf("concurrent edit digest %q: %v", r.sum, r.err)
		}
	case <-time.After(2 * time.Second):
		<-done
		t.Fatal("second request's scan blocked behind the open first scan")
	}
	// Scratch space owned by a prior scan cannot hide a subsequent edit.
	latest := &digestScan{}
	got, err := latest.sum(file)
	if err != nil || got != fmt.Sprintf("%x", sha256.Sum256(second)) {
		t.Fatalf("stale published digest %q: %v", got, err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if _, err := latest.sum(file); !os.IsNotExist(err) {
		t.Fatalf("removed source: %v", err)
	}
}

// ctime and mtime can both repeat across rapid writes. No timestamp needs to
// advance for an edit to count, even when a caller reuses its scan scratch space.
func TestDigestReadsRapidPreservedTimestampEdits(t *testing.T) {
	file := filepath.Join(t.TempDir(), "handler.go")
	stamp := time.Unix(1700000000, 0)
	scan := &digestScan{}
	for i := 0; i < 64; i++ {
		source := []byte(fmt.Sprintf("package fixture\nconst Value = %02d\n", i))
		if err := os.WriteFile(file, source, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(file, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		got, err := scan.sum(file)
		if want := fmt.Sprintf("%x", sha256.Sum256(source)); err != nil || got != want {
			t.Fatalf("edit %d digest %q, want %q: %v", i, got, want, err)
		}
	}
}
