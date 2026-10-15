package hotreload

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Artifact describes exactly the object the compiler produced. The generated
// host verifies this record before the loader reads or registers runtime data.
// The private HostView pins Identity; legacy Lower callers still check the
// qualified compiler/platform and object bytes.
type Artifact struct {
	Version, OS, Arch, Identity, Package, SHA256, Dependencies, CompilerProfile string
}

func (t Toolchain) writeArtifact(object, pkg, importcfg string) error {
	body, err := os.ReadFile(object)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	deps, err := dependencyIdentity(importcfg)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(Artifact{t.Version, t.OS, t.Arch, t.Identity, pkg, hex.EncodeToString(sum[:]), deps, "go1.27-native-v1"})
	if err != nil {
		return err
	}
	return os.WriteFile(object+".json", raw, 0644)
}

func (t Toolchain) checkFlags() error {
	for _, flag := range strings.Fields(os.Getenv("GOFLAGS")) {
		if strings.HasPrefix(flag, "-gcflags") || strings.HasPrefix(flag, "-tags") || strings.HasPrefix(flag, "-race") || strings.HasPrefix(flag, "-msan") || strings.HasPrefix(flag, "-asan") || strings.HasPrefix(flag, "-trimpath") {
			return fmt.Errorf("native development profile does not support GOFLAGS %s", flag)
		}
	}
	return nil
}

// Paths are not identities: stock cmd/go can choose a different cache path for
// equal archive bytes. Hash the captured dependency archives by import path.
func dependencyIdentity(importcfg string) (string, error) {
	raw, err := os.ReadFile(importcfg)
	if err != nil {
		return "", err
	}
	var entries []string
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		text := scanner.Text()
		if !strings.HasPrefix(text, "packagefile ") {
			continue
		}
		path, file, ok := strings.Cut(strings.TrimPrefix(text, "packagefile "), "=")
		if !ok {
			return "", fmt.Errorf("invalid compiler importcfg entry: %s", text)
		}
		body, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("compiler dependency %s: %w", path, err)
		}
		entries = append(entries, fmt.Sprintf("%s=%x", path, sha256.Sum256(body)))
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:]), nil
}
