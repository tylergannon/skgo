package gen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/tylergannon/polytype/grammar"
	"github.com/tylergannon/polytype/typegrammar"
	"github.com/tylergannon/skgo/internal/adapter"
	"github.com/tylergannon/skgo/internal/kithash"
	"github.com/tylergannon/skgo/templateapi"
	"golang.org/x/tools/imports"
)

func (a *app) generationModel(selections []string, marker string) (templateapi.GenerationModel, error) {
	var model templateapi.GenerationModel
	selected := map[string]bool{}
	for _, selection := range selections {
		if selected[selection] {
			return model, fmt.Errorf("skgo: duplicate generator selection %q", selection)
		}
		selected[selection] = true
	}
	var set codecSet
	index := func(typ types.Type, fn *remoteFn) int {
		if typ == nil || a.containsTransported(typ) {
			return -1
		}
		name := set.root(typ, fn.goPkg.loadDir, fn.pos)
		n, _ := strconv.Atoi(strings.TrimPrefix(name, "Root"))
		return n
	}
	for _, fn := range a.remotes {
		if marker != "" && !fn.markers[marker] {
			continue
		}
		selection := fn.module + "#" + fn.name
		if len(selections) > 0 && !selected[selection] {
			continue
		}
		delete(selected, selection)
		op := templateapi.RemoteOperation{Selection: selection, Name: fn.name, Module: fn.module, ID: kithash.Kit(fn.module) + "/" + fn.name, Kind: string(fn.kind), Input: -1, Output: -1, Source: fn.pos, WireFormat: "devalue"}
		op.Transported = a.containsTransported(fn.out) || fn.in != nil && a.containsTransported(fn.in)
		op.Description = fn.description
		if fn.kind == kindForm {
			op.WireFormat = "formdata"
		} else {
			op.Input = index(fn.in, fn)
		}
		op.Output = index(fn.out, fn)
		model.Remotes = append(model.Remotes, op)
	}
	if len(selected) > 0 {
		missing := make([]string, 0, len(selected))
		for name := range selected {
			missing = append(missing, name)
		}
		sort.Strings(missing)
		return model, fmt.Errorf("skgo: unknown generator remote selection(s): %s", strings.Join(missing, ", "))
	}
	var defs typegrammarDefs
	model.Roots = make([]typegrammar.Type, len(set.roots))
	for i, root := range set.roots {
		loaded, err := a.loadGrammar(root.loadDir)
		if err != nil {
			return model, err
		}
		lowered, roots, err := loaded.Lower([]grammar.Root{{Type: root.typ, Position: root.pos}})
		if err != nil {
			return model, fmt.Errorf("skgo: %s: generator wire model: %w", root.pos, err)
		}
		defs.add(lowered)
		model.Roots[i] = roots[0]
	}
	model.Definitions = defs.definitions
	return model, nil
}

// Ownership records contain hashes so an edited generated file becomes authored.
// Keeping them in the bindings directory also scopes ownership to this app.
type pluginOutputs map[string]map[string]string

func outputHash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func pluginOutputPath(root, relative string) (string, error) {
	if relative == "." || !path.IsAbs(relative) && path.Clean(relative) != relative || strings.Contains(relative, "\\") || path.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, "../") || relative == "" {
		return "", fmt.Errorf("skgo: invalid plugin output path %q", relative)
	}
	current := root
	for _, part := range strings.Split(relative, "/") {
		if part == ".git" || part == "node_modules" {
			return "", fmt.Errorf("skgo: invalid plugin output path %q", relative)
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("skgo: plugin output %q traverses a symlink", relative)
		}
	}
	return current, nil
}

func (a *app) runGenerators() error {
	if len(a.cfg.Generators) == 0 {
		return nil
	}
	root, mod, err := moduleOf(a.cfg.Out)
	if err != nil {
		return err
	}
	manifest := filepath.Join(a.cfg.Out, "skgo.plugin-outputs.json")
	records := pluginOutputs{}
	if info, err := os.Lstat(manifest); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("skgo: generator ownership record must be a regular file: %s", manifest)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if data, err := os.ReadFile(manifest); err == nil {
		if err := json.Unmarshal(data, &records); err != nil || records == nil {
			return fmt.Errorf("skgo: invalid generator ownership record %s", manifest)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	seenIDs := map[string]bool{}
	claimed := map[string]string{}
	for id, files := range records {
		for relative := range files {
			if owner, ok := claimed[relative]; ok && owner != id {
				return fmt.Errorf("skgo: conflicting ownership of plugin output %s", relative)
			}
			claimed[relative] = id
		}
	}
	ctx := a.cfg.Context
	if ctx == nil {
		ctx = context.Background()
	}
	remotePath := ""
	for _, inv := range a.cfg.Generators {
		if inv.ID == "" || inv.Generator == nil || seenIDs[inv.ID] {
			return fmt.Errorf("skgo: invalid or duplicate generator %q", inv.ID)
		}
		seenIDs[inv.ID] = true
		if err := ctx.Err(); err != nil {
			return err
		}
		marker := ""
		if g, ok := inv.Generator.(templateapi.MarkedGenerator); ok {
			marker = g.RemoteMarker()
			if parts := strings.Split(marker, "#"); len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return fmt.Errorf("skgo: generator %s has invalid remote marker %q", inv.ID, marker)
			}
		}
		model, err := a.generationModel(inv.Selections, marker)
		if err != nil {
			return err
		}
		options := map[string]string{}
		for k, v := range inv.Options {
			options[k] = v
		}
		projectName := a.cfg.ProjectName
		if projectName == "" {
			projectName = filepath.Base(root)
		}
		if provider, ok := inv.Generator.(templateapi.RemotePathGenerator); ok && provider.RequiresRemotePath() && remotePath == "" {
			remotePath, err = adapter.RemotePath(ctx, a.cfg.Web)
			if err != nil {
				return err
			}
		}
		result, err := inv.Generator.Generate(ctx, templateapi.GenerationRequest{RemoteMarker: marker, RemotePath: remotePath, Project: templateapi.Project{Root: root, Module: mod, Name: projectName, Web: a.cfg.Web}, Model: model, Selections: append([]string(nil), inv.Selections...), Options: options})
		if err != nil {
			return fmt.Errorf("skgo: generator %s: %w", inv.ID, err)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, d := range result.Diagnostics {
			switch d.Severity {
			case "warning":
				a.cfg.Logf("generator %s: %s: %s", inv.ID, d.Source, d.Message)
			case "error":
				return fmt.Errorf("skgo: generator %s: %s: %s", inv.ID, d.Source, d.Message)
			default:
				return fmt.Errorf("skgo: generator %s returned invalid diagnostic severity %q", inv.ID, d.Severity)
			}
		}
		next := map[string]string{}
		for _, file := range result.Files {
			target, err := pluginOutputPath(root, file.Path)
			if err != nil {
				return err
			}
			if withinTree(filepath.Join(a.cfg.Web, "src"), target) && generatedModuleName(filepath.Base(target)) {
				return fmt.Errorf("skgo: plugin %s cannot replace core frontend output %s", inv.ID, file.Path)
			}
			if filepath.Ext(target) == ".go" {
				formatted, err := imports.Process(target, file.Content, &imports.Options{Comments: true, TabIndent: true, TabWidth: 8, FormatOnly: true})
				if err != nil {
					return fmt.Errorf("skgo: plugin %s returned invalid Go output %s: %w", inv.ID, file.Path, err)
				}
				file.Content = formatted
			}
			if a.reservedPluginOutput(target, manifest) {
				return fmt.Errorf("skgo: plugin %s cannot replace core output %s", inv.ID, file.Path)
			}
			if _, ok := a.cfg.generation.files[target]; ok {
				return fmt.Errorf("skgo: duplicate generated output %s", file.Path)
			}
			if _, ok := next[file.Path]; ok {
				return fmt.Errorf("skgo: duplicate plugin output %s", file.Path)
			}
			if owner, ok := claimed[file.Path]; ok && owner != inv.ID {
				return fmt.Errorf("skgo: plugin output %s belongs to %s", file.Path, owner)
			}
			if err := checkPluginOutput(target, records[inv.ID][file.Path]); err != nil {
				return err
			}
			expected := records[inv.ID][file.Path]
			content := bytes.Clone(file.Content)
			relative := file.Path
			a.cfg.generation.preflight = append(a.cfg.generation.preflight, func() error {
				_, err := pluginOutputPath(root, relative)
				if err != nil {
					return err
				}
				return checkPluginOutput(target, expected)
			})
			a.cfg.generation.files[target] = string(content)
			if a.cfg.produced != nil {
				a.cfg.produced[target] = struct{}{}
			}
			next[file.Path] = outputHash(content)
			claimed[file.Path] = inv.ID
		}
		for relative, hash := range records[inv.ID] {
			if _, ok := next[relative]; ok {
				continue
			}
			target, err := pluginOutputPath(root, relative)
			if err != nil {
				return err
			}
			if a.reservedPluginOutput(target, manifest) {
				return fmt.Errorf("skgo: invalid plugin ownership of core output %s", relative)
			}
			if _, ok := a.cfg.generation.files[target]; ok {
				return fmt.Errorf("skgo: conflicting stale plugin output %s", relative)
			}
			if err := checkPluginOutput(target, hash); err != nil {
				return err
			}
			a.cfg.generation.preflight = append(a.cfg.generation.preflight, func() error {
				_, err := pluginOutputPath(root, relative)
				if err != nil {
					return err
				}
				return checkPluginOutput(target, hash)
			})
			if a.cfg.generation.removed == nil {
				a.cfg.generation.removed = map[string]bool{}
			}
			a.cfg.generation.removed[target] = true
			delete(claimed, relative)
		}
		records[inv.ID] = next
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	a.cfg.generation.files[manifest] = string(data) + "\n"
	a.cfg.generation.preflight = append(a.cfg.generation.preflight, ctx.Err)
	return nil
}

func checkPluginOutput(target, previous string) error {
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("skgo: %s is authored; refusing to overwrite it", target)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	if previous != "" && outputHash(data) == previous {
		return nil
	}
	return fmt.Errorf("skgo: %s is authored or edited; refusing to overwrite it", target)
}

// Link inventories and route boundaries are published after generation's file
// plan, so they must be reserved even when not present in that plan yet.
func (a *app) reservedPluginOutput(target, manifest string) bool {
	return target == manifest || filepath.Base(target) == generatedGoFile ||
		target == filepath.Join(a.cfg.Out, inventoryName) ||
		withinTree(filepath.Join(a.cfg.Out, linkRootName), target) ||
		target == filepath.Join(a.cfg.Web, "src", "routes", "go.mod")
}
