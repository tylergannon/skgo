package main

import (
	"context"
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
