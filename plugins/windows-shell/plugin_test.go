package main

import (
	"bytes"
	"context"
	"io/fs"
	"strings"
	"testing"

	tg "github.com/tylergannon/polytype/typegrammar"
	"github.com/tylergannon/skgo/plugins/windows-shell/sdk"
	"github.com/tylergannon/skgo/templateapi"
)

func TestWindowsPluginSurface(t *testing.T) {
	p := windowsPlugin{}
	d := p.Describe()
	if d.ID != "windows-shell" || !d.Generator || len(d.Addons) != 0 || len(d.Templates) != 0 {
		t.Fatalf("descriptor=%+v", d)
	}
	var help bytes.Buffer
	if err := p.RunCommand(t.Context(), templateapi.CommandRequest{Args: []string{"help"}, Stdout: &help}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(help.String(), "Apple") || strings.Contains(help.String(), "build") || !strings.Contains(help.String(), "windows/SDK") {
		t.Fatal(help.String())
	}
	for _, command := range []string{"setup", "prepare", "build"} {
		if err := p.RunCommand(t.Context(), templateapi.CommandRequest{Args: []string{command}}); err == nil {
			t.Fatalf("accepted %s", command)
		}
	}
	if _, err := fs.Stat(sdk.Resources(), "swift"); err == nil {
		t.Fatal("Windows SDK bundles Swift")
	}
	req := templateapi.GenerationRequest{Model: templateapi.GenerationModel{Roots: []tg.Type{&tg.Scalar{Kind: tg.String}}, Remotes: []templateapi.RemoteOperation{{Name: "read", ID: "kit-hash/read", Kind: "query", Input: -1, Output: 0}}}}
	got, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 1 || got.Files[0].Path != "windows/Generated.cs" || !strings.Contains(string(got.Files[0].Content), `"kit-hash/read"`) {
		t.Fatalf("generated=%+v", got)
	}
	for _, kind := range []string{"query.live", "query.batch", "form", "prerender"} {
		req.Model.Remotes[0].Kind = kind
		if _, err := p.Generate(t.Context(), req); err == nil {
			t.Fatalf("accepted %s", kind)
		}
	}
	req.Model.Remotes[0].Kind = "query"
	req.Model.Remotes[0].Transported = true
	if _, err := p.Generate(t.Context(), req); err == nil {
		t.Fatal("accepted custom transport")
	}
}
