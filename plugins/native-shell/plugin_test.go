package main

import (
	"context"
	"fmt"
	"github.com/tylergannon/skgo/plugins/native-shell/nativebuild"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tg "github.com/tylergannon/polytype/typegrammar"
	"github.com/tylergannon/skgo/templateapi"
)

func TestGeneratorUsesResolvedRemoteIdentityAndProtectsSupportedKinds(t *testing.T) {
	req := templateapi.GenerationRequest{Model: templateapi.GenerationModel{Roots: []tg.Type{&tg.Scalar{Kind: tg.String}}, Remotes: []templateapi.RemoteOperation{{Selection: "src/routes/value.remote.ts#readValue", Name: "readValue", ID: "kit-hash/readValue", Kind: "query", Input: -1, Output: 0}}}, Options: map[string]string{"out": "native/Value.swift"}}
	got, err := (nativePlugin{}).Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 1 || got.Files[0].Path != "native/Value.swift" || !strings.Contains(string(got.Files[0].Content), `core.call("kit-hash/readValue", kind: .query`) {
		t.Fatalf("resolved output: %+v", got)
	}
	req.Model.Remotes[0].Kind = "query.live"
	if _, err := (nativePlugin{}).Generate(context.Background(), req); err == nil {
		t.Fatal("accepted unsupported live query")
	}
	req.Model.Remotes[0].Kind = "query"
	req.Model.Remotes[0].Transported = true
	if _, err := (nativePlugin{}).Generate(context.Background(), req); err == nil {
		t.Fatal("accepted custom transport")
	}
}

func TestCSharpGeneratorIsRepeatableAndDefaultsSeparatelyFromSwift(t *testing.T) {
	req := templateapi.GenerationRequest{Model: templateapi.GenerationModel{Roots: []tg.Type{&tg.Scalar{Kind: tg.String}}, Remotes: []templateapi.RemoteOperation{{Name: "readValue", ID: "kit-hash/readValue", Kind: "query", Input: -1, Output: 0}}}, Options: map[string]string{"language": "csharp"}}
	first, err := (nativePlugin{}).Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := (nativePlugin{}).Generate(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Files) != 1 || first.Files[0].Path != "native/Generated.cs" || string(first.Files[0].Content) != string(second.Files[0].Content) || !strings.Contains(string(first.Files[0].Content), `core.CallAsync("kit-hash/readValue", RemoteClient.Kind.Query`) {
		t.Fatalf("C# output = %+v", first)
	}
	req.Options["language"] = "windows"
	if _, err := (nativePlugin{}).Generate(context.Background(), req); err == nil {
		t.Fatal("accepted unknown language")
	}
}

func TestSDKExportContainsCSharpSourcesAndPreservesCompilerOutputs(t *testing.T) {
	sdk, err := fs.Sub(resources, "sdk")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "SDK")
	if err := nativebuild.CopySDK(sdk, dest); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"csharp/SKGoNative/RemoteClient.cs", "csharp/RemoteCLI/Program.cs", "csharp/README.md", "swift/Package.swift", "core/build.zig"} {
		if _, err := os.Stat(filepath.Join(dest, path)); err != nil {
			t.Fatal(err)
		}
	}
	cache := filepath.Join(dest, "csharp", "SKGoNative", "obj", "sentinel")
	if err := os.MkdirAll(filepath.Dir(cache), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, []byte("compiler-owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := nativebuild.CopySDK(sdk, dest); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(cache); err != nil || string(data) != "compiler-owned" {
		t.Fatalf("cache changed: %s, %v", data, err)
	}
	if err := fs.WalkDir(resources, "sdk/csharp", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == "bin" || d.Name() == "obj" {
			return fmt.Errorf("embedded compiler output %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
