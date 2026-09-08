package skgo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/vite"
)

// fakeViteInfo starts a stub dev server answering `/__skgo_dev/info` with a
// manifest at devVersion 0 -- refreshDevLocked sees the version unchanged and
// returns immediately, so this exercises the real HTTP round trip through
// refreshDev/refreshDevLocked without needing a working Loads/Endpoints pair.
func fakeViteInfo(t *testing.T) *vite.Dev {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"entry":"entry.js","manifest":{"version":0}}`))
	}))
	t.Cleanup(server.Close)
	return vite.NewDev(server.URL)
}

// TestRefreshDevDoesNotBlockBehindAPendingRender locks down the fix for
// issue #110's dev-mode blocker: serveDev holds devMu for an async-pending
// render's whole duration (RLock, simulated here directly rather than through
// a full render), and refreshDev -- the callback every Go endpoint and load
// runs before dispatching, wired in NewDevPages -- must skip rather than
// block when that snapshot is held, leaving the next request to retry it.
// Before the fix, devMu was a plain Mutex taken for the same span, so this
// call would have blocked until the simulated render released it.
func TestRefreshDevDoesNotBlockBehindAPendingRender(t *testing.T) {
	s := &SSR{dev: &vite.Dev{}}

	renderHolds := make(chan struct{})
	releaseRender := make(chan struct{})
	go func() {
		s.devMu.RLock()
		defer s.devMu.RUnlock()
		close(renderHolds)
		<-releaseRender
	}()
	<-renderHolds
	defer close(releaseRender)

	done := make(chan error, 1)
	go func() { done <- s.refreshDev() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("refreshDev returned %v while a render held the snapshot", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("refreshDev blocked behind a pending render's read lock; other dev requests would stall until it finishes")
	}
}

// TestRefreshDevProceedsOnceNoRenderHoldsTheSnapshot is the negative control:
// the skip in refreshDev must be contingent on contention, not unconditional.
// Once nothing holds devMu, refreshDev must actually attempt the refresh
// (proven here by it reaching the dev server's HTTP call and failing to dial,
// rather than silently returning nil).
func TestRefreshDevProceedsOnceNoRenderHoldsTheSnapshot(t *testing.T) {
	s := &SSR{dev: vite.NewDev("http://127.0.0.1:1")}

	done := make(chan error, 1)
	go func() { done <- s.refreshDev() }()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "dev server") {
			t.Fatalf("refreshDev = %v, want a dial failure proving it attempted the refresh", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("refreshDev never returned")
	}
}

// TestDevPageRendersSerializeAgainstEachOther locks down a generation hazard
// the read-lock change above would otherwise open: each engine runtime
// refreshes its own module cache against vite independently of devMu
// (vite.Runner.Refresh, keyed by its own cursor), so two page renders in
// flight together could land on different runtimes at different vite
// generations while both read the one Go manifest snapshot devMu pins --
// pairing an old Go node/route table with a runtime that already picked up
// the new one. beginDevRender (the exact sequence serveDev holds for a
// render's whole duration) must keep renders one at a time, exactly as before
// a render could pend on slow Go I/O, and this must hold without
// reintroducing blocking for Go endpoints or loads: they only ever touch
// devMu directly (TryLock, skip on contention), never devRenderMu, which is
// what the concurrent refreshDev call below proves.
func TestDevPageRendersSerializeAgainstEachOther(t *testing.T) {
	s := &SSR{dev: fakeViteInfo(t)}

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		end, err := s.beginDevRender()
		if err != nil {
			t.Error(err)
			return
		}
		defer end()
		close(firstEntered)
		<-releaseFirst
	}()
	<-firstEntered

	// A concurrent Go endpoint/load request must still not block: it only
	// ever touches devMu (TryLock, skip on contention), never devRenderMu.
	endpointDone := make(chan error, 1)
	go func() { endpointDone <- s.refreshDev() }()
	select {
	case err := <-endpointDone:
		if err != nil {
			t.Fatalf("endpoint refreshDev returned %v while a render was in flight", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("endpoint refreshDev blocked behind an in-flight render; devRenderMu leaked into the endpoint path")
	}

	secondStarted := make(chan struct{})
	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		end, err := s.beginDevRender()
		if err != nil {
			t.Error(err)
			return
		}
		defer end()
		close(secondStarted)
	}()

	select {
	case <-secondStarted:
		t.Fatal("a second dev render started while the first was still pending; renders are no longer serialised")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseFirst)
	<-firstDone

	select {
	case <-secondStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("second render never started after the first released devRenderMu")
	}
	<-secondDone
}

func TestLiveDevManifestReplacesTheBuiltRouteAndNodeGraph(t *testing.T) {
	base := Manifest{
		Skgo:    "devel",
		AppDir:  "_app",
		Base:    "/base",
		Version: "built-version",
		Nodes:   []string{"old.server.ts"},
		Routes:  []ManifestRoute{{ID: "/old"}},
		SSR:     &ManifestSSR{Bundle: "bundle.js", GlobalName: "__sveltekit_built"},
	}
	answer := vite.Info{
		GlobalName: "__sveltekit_dev",
		Client:     vite.Client{Start: "/kit/start.js", App: "/generated/app.js", UsesEnvDynamicPublic: true},
		Manifest: vite.Manifest{
			Nodes:  []string{"layout.server.ts", "", "new/page.server.ts"},
			Routes: json.RawMessage(`[{"id":"/new","pattern":"^\\/new\\/?$","page":{"layouts":[0],"errors":[1],"leaf":2}}]`),
			SSR:    json.RawMessage(`{"assets":"","relative":true,"nodes":[{"index":0,"component":true,"ssr":null,"csr":null},{"index":1,"component":true,"ssr":null,"csr":null},{"index":2,"component":true,"ssr":null,"csr":null}]}`),
		},
	}

	live, err := manifestFromDev(base, answer)
	if err != nil {
		t.Fatal(err)
	}
	if live.Version != "" {
		t.Errorf("live version = %q, want empty so dev does not trigger build-version reloads", live.Version)
	}
	if len(live.Routes) != 1 || live.Routes[0].ID != "/new" {
		t.Fatalf("live routes = %#v, want only /new", live.Routes)
	}
	if got := live.Nodes; len(got) != 3 || got[2] != "new/page.server.ts" {
		t.Fatalf("live nodes = %#v", got)
	}
	if live.SSR == nil || len(live.SSR.Nodes) != 3 || live.SSR.Bundle != "" {
		t.Fatalf("live SSR = %#v", live.SSR)
	}
	if live.SSR.GlobalName != "__sveltekit_dev" || live.SSR.Client.App != "/generated/app.js" {
		t.Fatalf("live client = %#v under %q", live.SSR.Client, live.SSR.GlobalName)
	}
	if live.Base != base.Base || live.AppDir != base.AppDir || live.Skgo != base.Skgo {
		t.Fatal("live manifest discarded immutable build identity")
	}
}

func TestLiveDevManifestRejectsNodeTablesFromDifferentSnapshots(t *testing.T) {
	_, err := manifestFromDev(Manifest{}, vite.Info{
		Client: vite.Client{Start: "/start.js", App: "/app.js"},
		Manifest: vite.Manifest{
			Nodes:  []string{"one", "two"},
			Routes: json.RawMessage(`[]`),
			SSR:    json.RawMessage(`{"nodes":[{"index":0,"component":true}]}`),
		},
	})
	if err == nil {
		t.Fatal("a route snapshot and render-node snapshot with different lengths were accepted")
	}
}
