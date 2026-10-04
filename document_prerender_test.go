package skgo

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tylergannon/polytype/devalue"
	"github.com/tylergannon/skgo/internal/ssr"
)

func TestSSRUsesRecordedPrerenderRemoteAndKeepsOriginalHydrationKey(t *testing.T) {
	const module = "src/routes/prerender-consumer/consumer.remote.ts"
	var calls int
	fn := NewRemote(RemoteSpec{Kind: KindPrerender, Module: module, Name: "buildReceipt", Call: func(context.Context, Call) (any, error) {
		calls++
		return "live:atlas", nil
	}})
	id, payload := fn.ID(), "[\"atlas\"]"
	rel := "_app/remote/" + id + "/" + payload
	build := fstest.MapFS{
		"client/.keep":                  {Data: []byte("client")},
		"prerendered/" + rel:            {Data: artifactBytes(t, "result", "build:atlas")},
		"prerendered/index.html":        {Data: []byte("page")},
		"prerendered/_app/remote/stray": {Data: artifactBytes(t, "result", "stray")},
	}
	manifest := Manifest{AppDir: "_app", Prerendered: []string{"/" + rel}}
	remotes, err := NewRemotes(RemoteConfig{}, fn)
	if err != nil {
		t.Fatal(err)
	}
	store, err := newSSRPrerenderArtifactStore(build, manifest, remotes, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := &SSR{remotes: remotes, artifacts: store}
	answers := map[string]map[string]answered{}
	raw, err := s.answer(context.Background(), id, payload, answers)
	if err != nil {
		t.Fatal(err)
	}
	var answer remoteAnswer
	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatal(err)
	}
	if answer.V == "" || answer.E != nil || answer.H != nil || calls != 0 {
		t.Fatalf("answer=%s Go body calls=%d", raw, calls)
	}
	value, err := devalue.Parse(answer.V, nil)
	if err != nil || value != "build:atlas" {
		t.Fatalf("remote value = %#v, err=%v", value, err)
	}
	wantKey := id + "/" + payload
	if got := answers["p"][wantKey].tree; got != "build:atlas" {
		t.Fatalf("hydration p key %q = %#v", wantKey, got)
	}
	wire, err := s.remoteData(answers)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(wire, jsString(wantKey)) || !strings.Contains(wire, "build:atlas") {
		t.Fatalf("hydration data does not carry the original key/value: %s", wire)
	}
	if _, found, err := store.lookup(id, "stray", nil); err != nil || found {
		t.Fatalf("stray physical prerender file became authority: found=%v err=%v", found, err)
	}
}

func TestSSRPrerenderMissIsOpaqueAndNeverCallsGo(t *testing.T) {
	var calls int
	fn := NewRemote(RemoteSpec{Kind: KindPrerender, Module: "src/routes/consumer/consumer.remote.ts", Name: "value", Call: func(context.Context, Call) (any, error) {
		calls++
		return "live", nil
	}})
	remotes, err := NewRemotes(RemoteConfig{}, fn)
	if err != nil {
		t.Fatal(err)
	}
	s := &SSR{remotes: remotes}
	raw, err := s.answer(context.Background(), fn.ID(), "[\"missing\"]", map[string]map[string]answered{})
	if err != nil {
		t.Fatal(err)
	}
	var answer remoteAnswer
	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatal(err)
	}
	if calls != 0 || answer.E == nil || answer.E.Status != 500 || answer.E.Message != "Internal Error" {
		t.Fatalf("missing artifact answer=%s Go body calls=%d", raw, calls)
	}
}

func TestMalformedClientPrerenderErrorIsOpaqueAndNeverCallsGo(t *testing.T) {
	const module = "src/routes/consumer/consumer.remote.ts"
	var calls int
	fn := NewRemote(RemoteSpec{Kind: KindPrerender, Module: module, Name: "value", Call: func(context.Context, Call) (any, error) {
		calls++
		return "live", nil
	}})
	logical := "_app/remote/" + fn.ID() + "/payload"
	build := fstest.MapFS{"client/" + logical: {Data: []byte(`{"type":"error","error":{"status":"409","message":"broken"}}`)}}
	client, err := indexTree(build, "client")
	if err != nil {
		t.Fatal(err)
	}
	store, err := newPrerenderArtifactStore(build, Manifest{AppDir: "_app"}, client, map[string]assetMeta{})
	if err != nil {
		t.Fatal(err)
	}
	remotes, err := NewRemotes(RemoteConfig{}, fn)
	if err != nil {
		t.Fatal(err)
	}
	s := &SSR{remotes: remotes, artifacts: store}
	raw, err := s.answer(context.Background(), fn.ID(), "payload", map[string]map[string]answered{})
	if err != nil {
		t.Fatal(err)
	}
	var answer remoteAnswer
	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatal(err)
	}
	if calls != 0 || answer.E == nil || answer.E.Status != http.StatusInternalServerError || answer.E.Message != "Internal Error" {
		t.Fatalf("malformed client artifact answer=%s Go body calls=%d", raw, calls)
	}
}

func TestSSRPrerenderErrorIsAlreadyHandled(t *testing.T) {
	const errorBody = `{"status":409,"message":"built error","marker":"native-body"}`
	build, manifest, remotes, fn, rel := prerenderErrorBuild(t, errorBody)
	static, err := NewStaticHandler(build)
	if err != nil {
		t.Fatalf("NewStaticHandler for native App.Error: %v", err)
	}
	resp := do(t, static, http.MethodGet, "/"+rel, nil)
	if got := body(t, resp); resp.StatusCode != http.StatusOK || got != `{"type":"error","error":`+errorBody+`}` {
		t.Fatalf("HTTP remote artifact status=%d body=%q", resp.StatusCode, got)
	}
	s, err := NewSSR(build, manifest, nil, remotes, SSROptions{Runtimes: 1})
	if err != nil {
		t.Fatalf("NewSSR for native App.Error: %v", err)
	}
	answers := map[string]map[string]answered{}
	raw, err := s.answer(context.Background(), fn.ID(), "payload", answers)
	if err != nil {
		t.Fatal(err)
	}
	var answer remoteAnswer
	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatal(err)
	}
	if answer.H == nil || answer.H.Status != 409 || answer.H.Message != "built error" || answer.H.Extra["marker"] != "native-body" {
		t.Fatalf("built error envelope = %s", raw)
	}
	if got := answers["p"][fn.ID()+"/payload"].err; got == nil || got.Extra["marker"] != "native-body" {
		t.Fatalf("hydration error body = %#v", got)
	}
	if got := handleErrorAndJSONify(context.Background(), "", func(context.Context, CaughtError) map[string]any {
		t.Fatal("HandleError ran for Kit's already-handled body")
		return nil
	}, &HTTPError{Status: 500, Message: "Internal Error"}, &ssr.HandledError{Body: answer.H}, nil); got != answer.H {
		t.Fatalf("already-handled body changed: %#v", got)
	}
}
