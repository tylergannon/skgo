// Package nativeapp defines the native application configuration shared by the
// built-in native-app add-on, application plugins, and skgo native build.
package nativeapp

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"regexp"
)

// DefaultApplicationView is the exact replaceable application-owned Swift slot.
// An add-on must compare all bytes before replacing it, preserving user edits.
//
//go:embed files/native/Sources/ApplicationView.swift
var DefaultApplicationView string

// Config is stored at native/skgo-native.json. Paths are project-root relative.
// The build writes only disposable artifacts under native/build.
type Config struct {
	SwiftRemotes      []string          `json:"swiftRemotes,omitempty"`
	DeploymentTargets map[string]string `json:"deploymentTargets,omitempty"`
	Version           int               `json:"version"`
	BundleID          string            `json:"bundleId"`
	Packages          []Package         `json:"packages,omitempty"`
	Sources           []string          `json:"sources,omitempty"`
	Resources         []string          `json:"resources,omitempty"`
	Info              map[string]any    `json:"info,omitempty"`
	IOSInfo           map[string]any    `json:"iosInfo,omitempty"`
	MacOSInfo         map[string]any    `json:"macosInfo,omitempty"`
	Presets           map[string]Preset `json:"presets,omitempty"`
}

type Package struct {
	Name     string   `json:"name"`
	Path     string   `json:"path"`
	Products []string `json:"products"`
}

// Preset selects an output/bundle suffix and optional Go tags, Info.plist values,
// and resources. Swift packages remain linked for every preset. An omitted
// Resources slice inherits Config.Resources; an explicit empty slice clears it.
type Preset struct {
	GoTags       []string       `json:"goTags,omitempty"`
	BundleSuffix string         `json:"bundleSuffix,omitempty"`
	Info         map[string]any `json:"info,omitempty"`
	Resources    []string       `json:"resources"`
}

var bundleIdentifier = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*(\.[A-Za-z0-9][A-Za-z0-9-]*)+$`)
var presetName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Validate checks identity and contribution paths before a build changes files.
func (c Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("native configuration requires version 1")
	}
	if !bundleIdentifier.MatchString(c.BundleID) {
		return fmt.Errorf("invalid native bundleId %q", c.BundleID)
	}
	paths := append(append([]string{}, c.Sources...), c.Resources...)
	names := map[string]bool{"SKGoNative": true}
	for _, p := range c.Packages {
		if p.Name == "" || names[p.Name] {
			return fmt.Errorf("duplicate, reserved or empty native package %q", p.Name)
		}
		names[p.Name] = true
		paths = append(paths, p.Path)
	}
	for name, p := range c.Presets {
		if !presetName.MatchString(name) {
			return fmt.Errorf("invalid native preset name %q", name)
		}
		if p.BundleSuffix != "" && !bundleIdentifier.MatchString(c.BundleID+"."+p.BundleSuffix) {
			return fmt.Errorf("invalid bundle suffix %q", p.BundleSuffix)
		}
		paths = append(paths, p.Resources...)
	}
	for _, p := range paths {
		if !filepath.IsLocal(p) {
			return fmt.Errorf("native contribution path must be project-relative: %q", p)
		}
	}
	return nil
}
