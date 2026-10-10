// Package nativeapp defines the native application configuration shared by the
// built-in native-app add-on, application plugins, and skgo native build.
package nativeapp

import _ "embed"

// DefaultApplicationView is the exact replaceable application-owned Swift slot.
// An add-on must compare all bytes before replacing it, preserving user edits.
//
//go:embed files/native/Sources/ApplicationView.swift
var DefaultApplicationView string

// Config is stored at native/skgo-native.json. Paths are project-root relative.
// The build writes only disposable artifacts under native/build.
type Config struct {
	Version   int               `json:"version"`
	BundleID  string            `json:"bundleId"`
	Packages  []Package         `json:"packages,omitempty"`
	Sources   []string          `json:"sources,omitempty"`
	Resources []string          `json:"resources,omitempty"`
	Info      map[string]any    `json:"info,omitempty"`
	IOSInfo   map[string]any    `json:"iosInfo,omitempty"`
	MacOSInfo map[string]any    `json:"macosInfo,omitempty"`
	Presets   map[string]Preset `json:"presets,omitempty"`
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
