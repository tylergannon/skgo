package windowsapp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tylergannon/skgo/plugins/windows-shell/windowsapp"
	"github.com/tylergannon/skgo/templateapi"
)

func TestSetupPreservesApplicationAndRejectsConflicts(t *testing.T) {
	root := t.TempDir()
	r := templateapi.Request{Addon: "windows-app", Project: templateapi.Project{Root: root}, Options: map[string]string{"identity": "Example.Native"}}
	if _, err := windowsapp.Apply(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	view := filepath.Join(root, "windows/ApplicationView.cs")
	const authored = "// This application owns this view.\n"
	if err := os.WriteFile(view, []byte(authored), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := windowsapp.Apply(t.Context(), r)
	if err != nil || len(got.Changed) != 0 {
		t.Fatalf("repeat: %+v %v", got, err)
	}
	if b, _ := os.ReadFile(view); string(b) != authored {
		t.Fatal("authored view overwritten")
	}
	r.Options["identity"] = "Different.Native"
	if _, err := windowsapp.Apply(t.Context(), r); err == nil {
		t.Fatal("changed identity accepted")
	}
	other := t.TempDir()
	r.Project.Root = other
	if err := os.MkdirAll(filepath.Join(other, "windows"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "windows/ApplicationView.cs"), []byte(authored), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := windowsapp.Apply(t.Context(), r); err == nil {
		t.Fatal("unconfigured conflict accepted")
	}
	if _, err := os.Stat(filepath.Join(other, "windows/App.xaml")); !os.IsNotExist(err) {
		t.Fatal("partial write before conflict")
	}
}

func TestSetupRejectsSymlink(t *testing.T) {
	root, external := t.TempDir(), t.TempDir()
	if err := os.Symlink(external, filepath.Join(root, "windows")); err != nil {
		t.Fatal(err)
	}
	if _, err := windowsapp.Apply(t.Context(), templateapi.Request{Addon: "windows-app", Project: templateapi.Project{Root: root}, Options: map[string]string{"identity": "Example.Native"}}); err == nil {
		t.Fatal("symlink accepted")
	}
	entries, _ := os.ReadDir(external)
	if len(entries) != 0 {
		t.Fatal("wrote outside project")
	}
}
