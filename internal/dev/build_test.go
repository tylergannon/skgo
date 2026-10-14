package dev

import (
	"debug/dwarf"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestDevBuildRetainsSymbolsAndOffersDWARF(t *testing.T) {
	for _, tc := range []struct {
		name      string
		debug     bool
		flags     []string
		env       string
		wantDWARF bool
	}{
		{name: "fast default"},
		{name: "debug", debug: true, wantDWARF: true},
		{name: "caller linker flags", flags: []string{"-ldflags=-w=false"}, wantDWARF: true},
		{name: "caller GOFLAGS", env: "-ldflags=-w=false", wantDWARF: true},
		{name: "explicit omission in debug mode", debug: true, flags: []string{"-ldflags=-w"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOFLAGS", tc.env)
			s := startSessionConfigured(t, 20*time.Millisecond, func(cfg *Config) { cfg.Debug = tc.debug; cfg.BuildFlags = tc.flags })
			s.until("/", func(r reply) bool { return r.status == 200 && strings.HasPrefix(r.body, "revision one pid=") }, "literal application response")
			binary := filepath.Join(s.server.tmp, "app-1")
			data, err := binaryDWARF(binary)
			if tc.wantDWARF {
				if err != nil {
					t.Fatalf("debug data absent: %v", err)
				}
				entry, err := data.Reader().Next()
				if err != nil || entry == nil {
					t.Fatalf("empty debug data: %v", err)
				}
			} else if err == nil {
				if entry, _ := data.Reader().Next(); entry != nil {
					t.Fatal("fast development binary retained DWARF")
				}
			}
			symbols, err := exec.Command("go", "tool", "nm", binary).CombinedOutput()
			if err != nil || !strings.Contains(string(symbols), " main.main") {
				t.Fatalf("Go symbols removed: %v\n%s", err, symbols)
			}
		})
	}
}

func binaryDWARF(path string) (*dwarf.Data, error) {
	switch runtime.GOOS {
	case "darwin":
		f, err := macho.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return f.DWARF()
	case "windows":
		f, err := pe.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return f.DWARF()
	default:
		f, err := elf.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return f.DWARF()
	}
}
