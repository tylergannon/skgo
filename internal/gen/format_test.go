package gen

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestWriteFormatsGeneratedSourceBeforeComparing(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("the fixture formatter is a shell script")
	}
	web := t.TempDir()
	bin := filepath.Join(web, "node_modules", ".bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	vp := filepath.Join(bin, "vp")
	if err := os.WriteFile(vp, []byte("#!/bin/sh\n[ \"$1\" = fmt ] || exit 2\n[ \"$2\" = --stdin-filepath ] || exit 3\nsed 's/raw/formatted/g'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(web, "src", "types.ts")
	cfg := fixtureConfig(Config{Web: web, Logf: func(string, ...any) {}})
	if err := write(cfg, path, "raw\n"); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(first), "formatted\n"; got != want {
		t.Fatalf("generated source = %q, want formatter output %q", got, want)
	}
	firstInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := write(cfg, path, "raw\n"); err != nil {
		t.Fatal(err)
	}
	secondInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !firstInfo.ModTime().Equal(secondInfo.ModTime()) {
		t.Fatal("repeating generation rewrote already formatted source")
	}
}

func TestWriteWithoutVitePlusLeavesGenerationAvailable(t *testing.T) {
	t.Parallel()
	web := t.TempDir()
	path := filepath.Join(web, "src", "types.ts")
	if err := write(fixtureConfig(Config{Web: web, Logf: func(string, ...any) {}}), path, "raw\n"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "raw\n" {
		t.Fatalf("source without VitePlus = %q, want original content", got)
	}
}
