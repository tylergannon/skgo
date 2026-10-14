package dev

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
