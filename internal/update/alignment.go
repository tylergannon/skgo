package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tylergannon/skgo/internal/toolchain"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

type vpDependencies struct{ Core, Vitest string }

// The qualified global CLI reports its own bundled versions, independent of
// the old application's installation. Vite's upstream version is not the
// version of the vite-plus-core package used by its npm alias.
func parseVPToolchain(data []byte) (vpDependencies, error) {
	var graph struct {
		SchemaVersion int `json:"schemaVersion"`
		Source        struct {
			Scope, VitePlusVersion string
		} `json:"source"`
		Nodes []struct{ Name, Version string } `json:"nodes"`
	}
	if err := json.Unmarshal(data, &graph); err != nil {
		return vpDependencies{}, err
	}
	if graph.SchemaVersion != 1 || graph.Source.Scope != "global" || graph.Source.VitePlusVersion != toolchain.VitePlus {
		return vpDependencies{}, fmt.Errorf("expected global VitePlus %s toolchain graph version 1", toolchain.VitePlus)
	}
	var pins vpDependencies
	for _, n := range graph.Nodes {
		var target *string
		switch n.Name {
		case "@voidzero-dev/vite-plus-core":
			target = &pins.Core
		case "vitest":
			target = &pins.Vitest
		}
		if target != nil {
			if *target != "" || !semver.IsValid("v"+n.Version) {
				return vpDependencies{}, fmt.Errorf("invalid or duplicate %s version in vp toolchain", n.Name)
			}
			*target = n.Version
		}
	}
	if pins.Core == "" || pins.Vitest == "" {
		return vpDependencies{}, fmt.Errorf("vp toolchain did not report both vite-plus-core and vitest")
	}
	return pins, nil
}

func usesVitePlus(root string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return false, err
	}
	var pkg struct{ Dependencies, DevDependencies map[string]string }
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false, err
	}
	return pkg.Dependencies["vite-plus"] != "" || pkg.DevDependencies["vite-plus"] != "", nil
}

// Follow vp's documented manual update path: update declared versions and
// overrides together, then install. Unlike migrate, this never rewrites source,
// scripts, formatting, or first-time project setup.
func alignVPDependencies(root string, pins vpDependencies) error {
	name := filepath.Join(root, "package.json")
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	var pkg map[string]any
	if err := json.Unmarshal(data, &pkg); err != nil {
		return err
	}
	version := func(name string) string {
		switch name {
		case "vite-plus":
			return toolchain.VitePlus
		case "vite", "vite@*":
			return "npm:@voidzero-dev/vite-plus-core@" + pins.Core
		case "vitest", "vitest@*":
			return pins.Vitest
		}
		if vitestSibling(name) {
			return pins.Vitest
		}
		return ""
	}
	align := func(m map[string]any, references bool) {
		for name, old := range m {
			if v := version(name); v != "" {
				if ref, ok := old.(string); references && ok && strings.HasPrefix(ref, "catalog:") {
					continue
				}
				m[name] = v
			}
		}
	}
	for _, section := range []string{"dependencies", "devDependencies", "optionalDependencies", "peerDependencies"} {
		if deps, ok := pkg[section].(map[string]any); ok {
			align(deps, true)
		}
	}
	workspace := filepath.Join(root, "pnpm-workspace.yaml")
	data, err = os.ReadFile(workspace)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	config := map[string]any{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}
	if config == nil {
		config = map[string]any{}
	}
	if catalog, ok := config["catalog"].(map[string]any); ok {
		align(catalog, false)
	}
	if catalogs, ok := config["catalogs"].(map[string]any); ok {
		for _, value := range catalogs {
			if catalog, ok := value.(map[string]any); ok {
				align(catalog, false)
			}
		}
	}
	// pnpm uses package.json's overrides when present, ahead of workspace
	// overrides. Keep both existing locations aligned, and add the Vitest pin
	// only at the effective location. @* leaves catalog references intact.
	owner := config
	if pnpm, ok := pkg["pnpm"].(map[string]any); ok {
		if _, ok := pnpm["overrides"].(map[string]any); ok {
			owner = pnpm
		}
	}
	for _, location := range []map[string]any{config, owner} {
		if overrides, ok := location["overrides"].(map[string]any); ok {
			align(overrides, true)
		}
	}
	overrides, ok := owner["overrides"].(map[string]any)
	if !ok {
		overrides = map[string]any{}
		owner["overrides"] = overrides
	}
	if _, exists := overrides["vitest"]; !exists {
		overrides["vitest@*"] = pins.Vitest
	}
	data, err = json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(name, append(data, '\n'), 0644); err != nil {
		return err
	}
	data, err = yaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(workspace, data, 0644)
}

// These packages share Vitest's release version. Other packages in the scope,
// such as eslint-plugin and browser-webdriverio, release independently.
func vitestSibling(name string) bool {
	switch name {
	case "@vitest/browser", "@vitest/browser-playwright", "@vitest/browser-preview",
		"@vitest/coverage-v8", "@vitest/coverage-istanbul", "@vitest/ui",
		"@vitest/expect", "@vitest/mocker", "@vitest/pretty-format",
		"@vitest/runner", "@vitest/snapshot", "@vitest/spy", "@vitest/utils":
		return true
	}
	return false
}
