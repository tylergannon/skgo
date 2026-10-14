package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tylergannon/skgo/generate"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	cache, err := os.MkdirTemp("", "worker-capture-profile-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(cache)
	data, err := os.ReadFile("native/skgo-native.json")
	if err != nil {
		return err
	}
	var native struct {
		SwiftRemotes []string `json:"swiftRemotes"`
	}
	if err = json.Unmarshal(data, &native); err != nil {
		return err
	}
	cfg := generate.Config{Web: filepath.Join(root, "web"), Out: filepath.Join(root, "internal/skgo"), LocalsPackage: "github.com/tylergannon/gimbal/internal/app", HookPackage: "github.com/tylergannon/gimbal/internal/serverhooks", SwiftOut: filepath.Join(root, "native/Generated.swift"), SwiftRemotes: native.SwiftRemotes, CacheDir: cache}
	var worker generate.Worker
	defer worker.Close()
	measure := func(name string) error {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		start := time.Now()
		err := worker.Run(cfg)
		elapsed := time.Since(start)
		runtime.ReadMemStats(&after)
		fmt.Printf("MEASURE %s elapsed_ns=%d alloc_bytes=%d stats=%+v error=%v\n", name, elapsed.Nanoseconds(), after.TotalAlloc-before.TotalAlloc, worker.Stats(), err)
		return err
	}
	if err = measure("cold"); err != nil {
		return err
	}
	for i := 0; i < 3; i++ {
		if err = measure(fmt.Sprintf("warm%d", i)); err != nil {
			return err
		}
	}
	file := "web/src/routes/workflows/page.server.go"
	original, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	defer os.WriteFile(file, original, 0644)
	needle := []byte("func load(")
	start := bytes.Index(original, needle)
	if start < 0 {
		return fmt.Errorf("no Load function")
	}
	brace := bytes.IndexByte(original[start:], '{') + start + 1
	for i := 0; i < 8; i++ {
		changed := append([]byte{}, original[:brace]...)
		changed = append(changed, []byte(fmt.Sprintf("\n _ = \"capture-body-%d\"\n", i))...)
		changed = append(changed, original[brace:]...)
		if err = os.WriteFile(file, changed, 0644); err != nil {
			return err
		}
		if err = measure(fmt.Sprintf("body%d", i)); err != nil {
			return err
		}
	}
	if err = os.WriteFile(file, original, 0644); err != nil {
		return err
	}
	return worker.Run(cfg)
}
