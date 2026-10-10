package nativebuild

import (
	"encoding/json"
	"github.com/tylergannon/skgo/nativeapp"
	"reflect"
	"strings"
	"testing"
)

func TestApplicationNativeContributions(t *testing.T) {
	c := nativeapp.Config{Version: 1, BundleID: "dev.example.recorder", SwiftRemotes: []string{"src/lib/transcript.remote.ts#openSession"}, Packages: []nativeapp.Package{{Name: "RecorderTranscription", Path: "native/Transcription", Products: []string{"TranscriptSource", "AppleTranscription", "ScriptedTranscription"}}}, Resources: []string{"native/Resources"}, IOSInfo: map[string]any{"UIBackgroundModes": []string{"audio"}}, Info: map[string]any{"NSMicrophoneUsageDescription": "Record notes"}}
	p := nativeapp.Preset{BundleSuffix: "synthetic.burst", Info: map[string]any{"VNDefaultSource": "synthetic", "VNTranscriptFixture": "burst-transcript"}, Resources: []string{"native/Fixtures", "native/Resources/Licenses"}}
	spec, err := projectSpec("/app", "FieldNotes", "/app/native/build/simulator-synthetic-burst", "/sdk", "iOS", "26.0", c, p)
	if err != nil {
		t.Fatal(err)
	}
	target := spec["targets"].(map[string]any)["FieldNotes"].(map[string]any)
	info := target["info"].(map[string]any)["properties"].(map[string]any)
	if !reflect.DeepEqual(info["UIBackgroundModes"], []string{"audio"}) || info["NSMicrophoneUsageDescription"] != "Record notes" || info["VNTranscriptFixture"] != "burst-transcript" {
		t.Fatalf("plist properties: %#v", info)
	}
	settings := target["settings"].(map[string]any)["base"].(map[string]any)
	if settings["PRODUCT_BUNDLE_IDENTIFIER"] != "dev.example.recorder.synthetic.burst" {
		t.Fatalf("bundle settings: %#v", settings)
	}
	wantDeps := []any{map[string]any{"package": "SKGoNative"}, map[string]any{"package": "RecorderTranscription", "product": "TranscriptSource"}, map[string]any{"package": "RecorderTranscription", "product": "AppleTranscription"}, map[string]any{"package": "RecorderTranscription", "product": "ScriptedTranscription"}}
	if !reflect.DeepEqual(target["dependencies"], wantDeps) {
		t.Fatalf("package linkage: %#v", target["dependencies"])
	}
	if !reflect.DeepEqual(spec["packages"].(map[string]any)["RecorderTranscription"], map[string]any{"path": "/app/native/Transcription"}) {
		t.Fatal("wrong package path")
	}
	b, _ := json.Marshal(spec)
	s := string(b)
	for _, want := range []string{"dev.example.recorder.synthetic.burst", "/app/native/Sources", "/app/native/Generated.swift", "/app/native/Transcription", "TranscriptSource", "AppleTranscription", "ScriptedTranscription", "NSMicrophoneUsageDescription", "UIBackgroundModes", "burst-transcript", "/app/native/Fixtures", "/app/native/Resources/Licenses"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %s: %s", want, b)
		}
	}
	for _, resources := range [][]string{nil, {}} {
		p.Resources = resources
		spec, err = projectSpec("/app", "FieldNotes", "/build", "/sdk", "iOS", "26.0", c, p)
		if err != nil {
			t.Fatal(err)
		}
		b, _ = json.Marshal(spec)
		has := strings.Contains(string(b), `"path":"/app/native/Resources"`)
		if has != (resources == nil) {
			t.Fatalf("nil/empty resource semantics: %s", b)
		}
	}
	c.Sources = []string{"../outside.swift"}
	if _, err := projectSpec("/app", "FieldNotes", "/build", "/sdk", "iOS", "26.0", c, p); err == nil {
		t.Fatal("accepted escaping contribution")
	}
}

func TestInvalidNativeIdentityAndPaths(t *testing.T) {
	for _, c := range []nativeapp.Config{
		{Version: 1, BundleID: "dev.bad_name.app"},
		{Version: 1, BundleID: "dev..app"},
		{Version: 1, BundleID: "dev.example.app", Presets: map[string]nativeapp.Preset{"real": {BundleSuffix: ".bad"}}},
		{Version: 1, BundleID: "dev.example.app", Presets: map[string]nativeapp.Preset{"real": {BundleSuffix: "bad_suffix"}}},
		{Version: 1, BundleID: "dev.example.app", Resources: []string{"../outside"}},
	} {
		if err := c.Validate(); err == nil {
			t.Fatalf("accepted invalid config %+v", c)
		}
	}
}
