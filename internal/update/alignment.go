package update

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return err
	}
	if err := document.Decode(&config); err != nil {
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
	// The qualified pnpm 12 companion reads overrides only from the workspace
	// file. Legacy package.json pnpm settings are ignored; leave those authored
	// values untouched. @* leaves catalog references intact.
	overrides, ok := config["overrides"].(map[string]any)
	if !ok {
		overrides = map[string]any{}
		config["overrides"] = overrides
	}
	align(overrides, true)
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
	if err := alignYAMLValues(&document, config); err != nil {
		return err
	}
	data, err = yaml.Marshal(&document)
	if err != nil {
		return err
	}
	return os.WriteFile(workspace, data, 0644)
}

// Update changed scalar values and append new mapping entries in the parsed
// document, retaining existing comments, key order and scalar quoting.
func alignYAMLValues(node *yaml.Node, value any) error {
	if node.Kind == yaml.DocumentNode {
		return alignYAMLValues(node.Content[0], value)
	}
	mapping, ok := value.(map[string]any)
	if node.Kind != yaml.MappingNode || !ok {
		var previous any
		if err := node.Decode(&previous); err != nil {
			return err
		}
		if reflect.DeepEqual(previous, value) {
			return nil
		}
		if text, ok := value.(string); ok && node.Kind == yaml.ScalarNode {
			node.Value, node.Tag = text, "!!str"
			return nil
		}
		if node.Kind == 0 {
			return node.Encode(value)
		}
		return fmt.Errorf("cannot align dependency YAML node at line %d without replacing authored structure", node.Line)
	}
	remaining := make(map[string]any, len(mapping))
	for k, v := range mapping {
		remaining[k] = v
	}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if v, ok := remaining[key]; ok {
			if err := alignYAMLValues(node.Content[i+1], v); err != nil {
				return err
			}
			delete(remaining, key)
		}
	}
	keys := make([]string, 0, len(remaining))
	for key := range remaining {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		var child yaml.Node
		if err := child.Encode(remaining[key]); err != nil {
			return err
		}
		node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &child)
	}
	return nil
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
