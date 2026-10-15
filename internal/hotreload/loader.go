package hotreload

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The exact purego revision used by the no-cgo experiment. It stays a private
// host dependency; SDK/CLI code only embeds the small adapter source files.
const PuregoVersion = "v0.12.0-alpha.1.0.20261009162626-eafd1cca3a52"

//go:embed loaderos/dl_unix.go.txt loaderos/mmap_darwin_arm64.go.txt loaderos/typelink_go127.go.txt
var loaderOS embed.FS

// PrepareLoader materializes the revision-locked upstream loader with no-cgo
// OS adapters and numeric retirement integration. dir is a private host module, never authored
// application source. Its dependency declaration points at this private copy.
func (t Toolchain) PrepareLoader(ctx context.Context, dir, parent string) (string, error) {
	cmd := t.command(ctx, dir, "mod", "download", "-json", "github.com/pkujhd/goloader@"+LoaderVersion)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("download native loader: %w\n%s", err, raw)
	}
	var module struct {
		Dir, Version string
		Error        *struct{ Err string }
	}
	if err := json.Unmarshal(raw, &module); err != nil {
		return "", err
	}
	if module.Error != nil {
		return "", fmt.Errorf("download native loader: %s", module.Error.Err)
	}
	if module.Dir == "" || module.Version != LoaderVersion {
		return "", fmt.Errorf("unexpected native loader source: %s", raw)
	}
	dest := filepath.Join(parent, t.Identity, "loader")
	err = filepath.WalkDir(module.Dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(module.Dir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, 0644)
	})
	if err != nil {
		return "", fmt.Errorf("materialize native loader: %w", err)
	}
	for _, adapter := range []struct{ source, target string }{
		{"dl_unix.go.txt", "libdl/dl_unix.go"},
		{"mmap_darwin_arm64.go.txt", "mmap/mmap_darwin_arm64.go"},
		{"typelink_go127.go.txt", "link/typelink.1.27.go"},
	} {
		body, err := loaderOS.ReadFile("loaderos/" + adapter.source)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dest, adapter.target), body, 0644); err != nil {
			return "", err
		}
	}
	linkerPath := filepath.Join(dest, "link/ld.go")
	linkerSource, err := os.ReadFile(linkerPath)
	if err != nil {
		return "", err
	}
	const original = "type Linker struct {"
	if strings.Count(string(linkerSource), original) != 1 {
		return "", fmt.Errorf("native loader type differs from pinned source")
	}
	linkerSource = []byte(strings.Replace(string(linkerSource), original, "type Linker struct {\nReclaimable bool", 1))
	if err := os.WriteFile(linkerPath, linkerSource, 0644); err != nil {
		return "", err
	}
	// Go's runtime uses ^uint32(0) for absent funcdata. Upstream used zero
	// internally, then relocated that zero with type/itab backing: functions
	// without open-coded defers acquired a false defer-info pointer and a
	// replacement panic killed the process with "missing deferreturn".
	// Preserve an explicit sentinel, including real metadata at offset zero.
	for _, patch := range []struct{ file, old, next string }{
		{"link/ld.go", "Func.FuncData = append(Func.FuncData, (uintptr)(0))", "Func.FuncData = append(Func.FuncData, ^uintptr(0))"},
		{"link/ld.go", "sym.Func.FuncData[index] += uintptr(segment.noPtrTypeDataLen + segment.noPtrItabDataLen)", "if sym.Func.FuncData[index] != ^uintptr(0) { sym.Func.FuncData[index] += uintptr(segment.noPtrTypeDataLen + segment.noPtrItabDataLen) }"},
		{"link/inlinetree.go", "sym.Func.FuncData = append(sym.Func.FuncData, uintptr(0))", "sym.Func.FuncData = append(sym.Func.FuncData, ^uintptr(0))"},
		{"link/functab.1.18.go", "if v != 0 {", "if v != ^uintptr(0) {"},
		{"stackobject/stackobject.go", "Func.FuncData[dataindex.FUNCDATA_StackObjects] != 0", "Func.FuncData[dataindex.FUNCDATA_StackObjects] != ^uintptr(0)"},
	} {
		file := filepath.Join(dest, patch.file)
		body, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		if !strings.Contains(string(body), patch.old) {
			return "", fmt.Errorf("native loader funcdata differs from pinned source: %s", patch.file)
		}
		body = []byte(strings.ReplaceAll(string(body), patch.old, patch.next))
		if err := os.WriteFile(file, body, 0644); err != nil {
			return "", err
		}
	}
	run := func(dir string, args ...string) error {
		output, err := t.command(ctx, dir, args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("prepare native loader %v: %w\n%s", args, err, output)
		}
		return nil
	}
	if err := run(dest, "mod", "edit", "-require=github.com/ebitengine/purego@"+PuregoVersion); err != nil {
		return "", err
	}
	if err := run(dir, "mod", "edit", "-require=github.com/pkujhd/goloader@"+LoaderVersion, "-replace=github.com/pkujhd/goloader="+dest); err != nil {
		return "", err
	}
	return dest, nil
}
