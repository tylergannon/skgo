package nativeapp

import (
	"context"
	"github.com/tylergannon/skgo/templateapi"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeApplicationPreservesOwnedSlotsAndRefusesConflicts(t *testing.T) {
	root := t.TempDir()
	p := Plugin("v1.0.0")
	req := templateapi.Request{Addon: "native-app", Project: templateapi.Project{Root: root, Module: "example.test/field", Name: "FieldNotes", Web: filepath.Join(root, "web")}, Options: map[string]string{"bundle-id": "dev.example.field"}}
	r, err := p.Apply(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Changed) != 8 {
		t.Fatalf("changed=%v", r.Changed)
	}
	host, err := os.ReadFile(filepath.Join(root, "native/host/main.go"))
	if err != nil || !strings.Contains(string(host), `app "example.test/field"`) {
		t.Fatalf("host: %s %v", host, err)
	}
	slot := filepath.Join(root, "native/Sources/ApplicationView.swift")
	if b, _ := os.ReadFile(slot); string(b) != DefaultApplicationView {
		t.Fatal("default slot differs from public replacement bytes")
	}
	os.WriteFile(slot, []byte("// application owned\n"), 0644)
	config := filepath.Join(root, "native/skgo-native.json")
	custom := `{"version":1,"bundleId":"dev.example.field","iosInfo":{"UIBackgroundModes":["audio"]}}`
	os.WriteFile(config, []byte(custom), 0644)
	r, err = p.Apply(context.Background(), req)
	if err != nil || len(r.Changed) != 0 {
		t.Fatalf("repeat: %+v %v", r, err)
	}
	if b, _ := os.ReadFile(slot); string(b) != "// application owned\n" {
		t.Fatal("lost application edit")
	}
	if b, _ := os.ReadFile(config); string(b) != custom {
		t.Fatal("lost configuration")
	}
	stable := filepath.Join(root, "native/Sources/NativeHost.swift")
	os.WriteFile(stable, []byte("// edited shell\n"), 0644)
	os.Remove(filepath.Join(root, "native/Bridge.h"))
	r, err = p.Apply(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "conflicts") || len(r.Changed) != 0 {
		t.Fatalf("conflict: %+v %v", r, err)
	}
	if _, err := os.Stat(filepath.Join(root, "native/Bridge.h")); !os.IsNotExist(err) {
		t.Fatalf("partial mutation before conflict %v", err)
	}
}
