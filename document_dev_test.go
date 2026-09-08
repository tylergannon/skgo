package skgo

import (
	"encoding/json"
	"testing"

	"github.com/tylergannon/skgo/internal/vite"
)

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
