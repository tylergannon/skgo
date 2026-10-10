package templates

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/templateapi"
)

type fixture struct {
	d     templateapi.Descriptor
	calls *[]templateapi.Request
	fail  string
}

func (f fixture) Describe() templateapi.Descriptor { return f.d }
func (f fixture) Apply(_ context.Context, r templateapi.Request) (templateapi.Result, error) {
	*f.calls = append(*f.calls, r)
	if r.Addon == f.fail {
		return templateapi.Result{Changed: []string{"partial.txt"}}, errors.New("intentional failure")
	}
	return templateapi.Result{}, nil
}
func fixtureRegistry(t *testing.T, fs ...fixture) *Registry {
	t.Helper()
	var plugins []templateapi.Plugin
	for _, f := range fs {
		plugins = append(plugins, f)
	}
	r, err := Discover("", t.TempDir(), t.TempDir(), "test", plugins...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestRecipeAndManualAddResolveTheSameRequests(t *testing.T) {
	var calls []templateapi.Request
	p := fixture{d: templateapi.Descriptor{ID: "fixture", Version: "1", SkgoVersion: "test", Addons: []templateapi.Addon{{Name: "first", Summary: "First", Options: []templateapi.Option{{Name: "label", Help: "Label", Required: true}}}, {Name: "second", Summary: "Second"}}, Templates: []templateapi.Template{{Name: "example", Summary: "Example", Options: []templateapi.Option{{Name: "title", Help: "Title", Default: "Original"}}, Steps: []templateapi.Step{{Addon: "first", Bindings: map[string]string{"label": "title"}}, {Addon: "second"}}}}}, calls: &calls}
	r := fixtureRegistry(t, p)
	ops, err := r.Template("example", map[string]string{"title": "Changed"})
	if err != nil {
		t.Fatal(err)
	}
	project := templateapi.Project{Root: "/literal", Name: "Independent", Module: "example.test/independent", Web: "/literal/web"}
	if err := Apply(context.Background(), ops, project, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	want := []templateapi.Request{{Addon: "first", Project: project, Options: map[string]string{"label": "Changed"}}, {Addon: "second", Project: project, Options: map[string]string{}}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("got %#v want %#v", calls, want)
	}
	calls = nil
	for _, v := range want {
		ops, err := r.Add(v.Addon, v.Options)
		if err != nil {
			t.Fatal(err)
		}
		if err := Apply(context.Background(), ops, project, &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("manual=%#v", calls)
	}
	p.fail = "first"
	r = fixtureRegistry(t, p)
	ops, _ = r.Template("example", nil)
	calls = nil
	var output bytes.Buffer
	if err := Apply(context.Background(), ops, project, &output); err == nil || !strings.Contains(err.Error(), "step 1/2 (first) failed after 0 completed") {
		t.Fatalf("failure=%v", err)
	}
	if len(calls) != 1 || !strings.Contains(output.String(), "partial.txt") {
		t.Fatalf("calls=%v output=%s", calls, output.String())
	}
}
func TestInvalidRecipeAndAmbiguityNeverApply(t *testing.T) {
	var calls []templateapi.Request
	base := fixture{d: templateapi.Descriptor{ID: "a", Version: "1", SkgoVersion: "test", Addons: []templateapi.Addon{{Name: "x", Summary: "X", Options: []templateapi.Option{{Name: "required", Help: "Required", Required: true}}}}}, calls: &calls}
	r := fixtureRegistry(t, base)
	for _, values := range []map[string]string{nil, {"unexpected": "x"}} {
		if _, err := r.Add("x", values); err == nil {
			t.Fatalf("accepted %v", values)
		}
	}
	other := base
	other.d.ID = "b"
	r = fixtureRegistry(t, base, other)
	if _, err := r.Add("x", map[string]string{"required": "yes"}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("err=%v", err)
	}
	other.d.ID = "a"
	r = fixtureRegistry(t, base, other)
	if len(r.Entries) != 0 || len(r.Diagnostics) != 1 {
		t.Fatalf("duplicate ID registry=%+v", r)
	}
	if len(calls) != 0 {
		t.Fatalf("called Apply: %v", calls)
	}
}
func TestDirectorySelectionAndIdentityPreserveFiles(t *testing.T) {
	home, cwd := t.TempDir(), t.TempDir()
	if _, err := Discover("", home, cwd, "test"); err != nil {
		t.Fatal(err)
	}
	for _, setting := range []string{"missing", "one,,two", ","} {
		if _, err := Discover(setting, home, cwd, "test"); err == nil {
			t.Fatalf("accepted %q", setting)
		}
	}
	plugins := filepath.Join(home, "plugins")
	if err := os.Mkdir(plugins, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover("~/plugins,"+plugins, home, cwd, "test"); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "web"), 0755)
	os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/independent\n\ngo 1.27.1\n"), 0644)
	if _, _, err := Project(root, ""); err == nil {
		t.Fatal("older project silently inferred name")
	}
	p, missing, err := Project(root, "OwnName")
	if err != nil || !missing {
		t.Fatalf("%+v %v %v", p, missing, err)
	}
	if err := SaveIdentity(p); err != nil {
		t.Fatal(err)
	}
	if err := SaveIdentity(p); err == nil {
		t.Fatal("overwrote identity")
	}
	if _, _, err := Project(root, "OtherName"); err == nil {
		t.Fatal("silently renamed app")
	}
	got, missing, err := Project(root, "")
	if err != nil || missing || got.Name != "OwnName" || got.Module != "example.test/independent" {
		t.Fatalf("%+v %v %v", got, missing, err)
	}
}
