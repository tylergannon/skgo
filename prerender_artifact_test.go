package skgo

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/tylergannon/polytype/devalue"
)

type artifactMoney struct{ Cents int }

func artifactData(t *testing.T, value any) string {
	t.Helper()
	wire, err := devalue.Stringify(devalue.NewObject("_", value))
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

func artifactBytes(t *testing.T, kind string, value any) []byte {
	t.Helper()
	var body any
	switch kind {
	case "result", "redirect":
		body = map[string]any{"type": kind, "data": artifactData(t, value)}
	case "error":
		body = map[string]any{"type": kind, "error": value}
	default:
		t.Fatalf("unsupported artifact fixture type %q", kind)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestPrerenderArtifactLookupUsesClientFirstAndManifestAuthority(t *testing.T) {
	const id = "abc123/buildReceipt"
	const payload = "[\"atlas\"]"
	remotePath := "_app/remote/" + id + "/" + payload
	build := fstest.MapFS{
		"client/" + remotePath:                              {Data: artifactBytes(t, "result", "client:atlas")},
		"prerendered/" + remotePath:                         {Data: artifactBytes(t, "result", "build:atlas")},
		"prerendered/_app/remote/abc123/buildReceipt/stray": {Data: artifactBytes(t, "result", "stray")},
		"index.html":         {Data: []byte(testIndexHTML)},
		"skgo.manifest.json": {Data: []byte(testManifest)},
	}
	client, err := indexTree(build, "client")
	if err != nil {
		t.Fatal(err)
	}
	prerendered, err := indexTree(build, "prerendered")
	if err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{AppDir: "_app", Base: "/mount", Prerendered: []string{"/mount/" + remotePath}}
	store, err := newPrerenderArtifactStore(build, manifest, client, prerendered)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := store.lookup(id, payload, nil)
	if err != nil || !found {
		t.Fatalf("lookup: found=%v err=%v", found, err)
	}
	if got.result != "client:atlas" {
		t.Fatalf("client asset did not shadow the recorded page artifact: %#v", got.result)
	}
	if _, found, err := store.lookup(id, "stray", nil); err != nil || found {
		t.Fatalf("unlisted prerender file gained authority: found=%v err=%v", found, err)
	}
	if _, found, err := store.lookup(id, "[\"missing\"]", nil); err != nil || found {
		t.Fatalf("unknown artifact lookup: found=%v err=%v", found, err)
	}
}

func TestPrerenderArtifactLookupAllowsClientOnlyAndNoArgumentNativeKeys(t *testing.T) {
	const id = "abc123/buildReceipt"
	clientPath := "_app/remote/" + id
	build := fstest.MapFS{"client/" + clientPath: {Data: artifactBytes(t, "result", "client-only")}}
	client, err := indexTree(build, "client")
	if err != nil {
		t.Fatal(err)
	}
	store, err := newPrerenderArtifactStore(build, Manifest{AppDir: "_app"}, client, map[string]assetMeta{})
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := store.lookup(id, "", nil)
	if err != nil || !found || got.result != "client-only" {
		t.Fatalf("no-argument client artifact: %#v found=%v err=%v", got.result, found, err)
	}
}

func TestPrerenderArtifactSyntaxAndNativeRedirectSemantics(t *testing.T) {
	if err := validatePrerenderArtifactSyntax([]byte(`{"type":"redirect","location":"/built-redirect","status":307,"data":"[{}]"}`), "redirect-fixture"); err != nil {
		t.Fatalf("native redirect envelope rejected: %v", err)
	}
	artifact, err := decodePrerenderArtifact([]byte(`{"type":"redirect","location":"/built-redirect","status":307,"data":"[{}]"}`), "redirect-fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := artifact.result.(devalue.UndefinedValue); !ok {
		t.Fatalf("missing result slot = %#v (%T), want JS undefined", artifact.result, artifact.result)
	}
	if err := validatePrerenderArtifactSyntax([]byte(`{"type":"result","data":"["}`), "broken"); err == nil {
		t.Fatal("malformed data JSON was accepted")
	}
	if _, err := decodePrerenderArtifact([]byte(`{"type":"result","data":"[0]"}`), "dangling-reference", nil); err == nil {
		t.Fatal("dangling devalue reference was accepted")
	}
}

func TestSSRArtifactUsesResultTransportReviverAtConstruction(t *testing.T) {
	transport := Transport{"Money": {
		Type: reflect.TypeOf(artifactMoney{}),
		Encode: func(value any) (any, error) {
			return []any{float64(value.(artifactMoney).Cents)}, nil
		},
		Decode: func(value any) (any, error) {
			values := value.([]any)
			return artifactMoney{Cents: int(values[0].(float64))}, nil
		},
	}}
	wire, err := devalue.StringifyWith(devalue.NewObject("_", artifactMoney{Cents: 750}), transport.reducers())
	if err != nil {
		t.Fatal(err)
	}
	fn := NewRemote(RemoteSpec{Kind: KindPrerender, Module: "src/routes/price/price.remote.ts", Name: "price", Call: func(context.Context, Call) (any, error) { return nil, nil }})
	rel := "_app/remote/" + fn.ID() + "/atlas"
	build := fstest.MapFS{
		"client/.keep":       {Data: []byte("client")},
		"prerendered/" + rel: {Data: []byte(`{"type":"result","data":` + mustJSON(t, wire) + `}`)},
	}
	remotes, err := NewRemotes(RemoteConfig{Transport: transport}, fn)
	if err != nil {
		t.Fatal(err)
	}
	store, err := newSSRPrerenderArtifactStore(build, Manifest{AppDir: "_app", Prerendered: []string{"/" + rel}}, remotes, transport)
	if err != nil {
		t.Fatal(err)
	}
	artifact, found, err := store.lookup(fn.ID(), "atlas", transport)
	if err != nil || !found {
		t.Fatalf("lookup found=%v err=%v", found, err)
	}
	if got, ok := artifact.result.(artifactMoney); !ok || got.Cents != 750 {
		t.Fatalf("result = %#v (%T), want transported artifactMoney{750}", artifact.result, artifact.result)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestPrerenderArtifactStoreRejectsMissingAndMalformedRecordedFiles(t *testing.T) {
	const id = "abc123/buildReceipt"
	path := "_app/remote/" + id + "/payload"
	manifest := Manifest{AppDir: "_app", Prerendered: []string{"/" + path}}
	for name, data := range map[string][]byte{"missing": nil, "malformed": []byte(`{"type":"result","data":"["}`)} {
		t.Run(name, func(t *testing.T) {
			build := fstest.MapFS{}
			if data != nil {
				build["prerendered/"+path] = &fstest.MapFile{Data: data}
			}
			prerendered := map[string]assetMeta{}
			if data != nil {
				var err error
				prerendered, err = indexTree(build, "prerendered")
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := newPrerenderArtifactStore(build, manifest, map[string]assetMeta{}, prerendered); err == nil {
				t.Fatal("invalid recorded artifact was accepted")
			}
		})
	}
}
