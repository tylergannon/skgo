package adapter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Kit invokes reroute through event.fetch before handle creates a logical owner.
// Exercise the shipped build callback at that point, including a nonempty base.
func TestPrerenderRerouteFetchPrecedesLogicalRequest(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, ".svelte-kit/output/server/manifest-full.js")
	if err := os.MkdirAll(filepath.Dir(manifest), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"type":"module"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, []byte(`export const manifest={app_dir:'_app',app_path:'base/_app',routes:[]};`), 0600); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer literal-reroute-secret" {
			http.Error(w, "missing auth", 401)
			return
		}
		if r.URL.Path != "/reroute" {
			http.Error(w, "no logical request exists before reroute", 409)
			return
		}
		var input struct {
			URL      string      `json:"url"`
			Headers  http.Header `json:"headers"`
			Manifest struct {
				Base   string `json:"base"`
				AppDir string `json:"appDir"`
			} `json:"manifest"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			http.Error(w, "bad JSON", 400)
			return
		}
		if input.URL != "http://app.test/base/alias?literal=381" || strings.Join(input.Headers["x-literal"], ",") != "request-571" || input.Manifest.Base != "/base" || input.Manifest.AppDir != "_app" {
			t.Errorf("reroute input: %+v", input)
			http.Error(w, "wrong input", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"path":"/base/literal-selected-281"}`)
	}))
	defer server.Close()
	module, err := filepath.Abs("skgo-adapter/prerender.js")
	if err != nil {
		t.Fatal(err)
	}
	const program = `import assert from 'node:assert/strict';
import {pathToFileURL} from 'node:url';
process.chdir(process.argv[2]);
const {requestFetch}=await import(pathToFileURL(process.argv[1]).href);
const event={url:new URL('http://app.test/base/alias?literal=381'),request:new Request('http://app.test/base/alias?literal=381',{headers:{'x-literal':'request-571'}}),route:{id:null},params:{},locals:{}};
const request=new Request('http://app.test/base/_app/reroute?url='+encodeURIComponent(event.url.href));
const response=await requestFetch({event,request,fetch:()=>{throw new Error('framework reroute transport entered Kit continuation')}});
assert.equal(response.status,200);
assert.deepEqual(await response.json(),{path:'/base/literal-selected-281'});`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	web, err := filepath.Abs("../../example/web")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, filepath.Join(web, "node_modules/.bin/vp"), "exec", "node", "--input-type=module", "-e", program, module, dir)
	command.Dir = web
	command.Env = append(os.Environ(), "SKGO_PRERENDER_SERVICE_URL="+server.URL, "SKGO_PRERENDER_SERVICE_SECRET=literal-reroute-secret")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("pre-handle reroute callback: %v\n%s", err, output)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 1 || paths[0] != "/reroute" {
		t.Fatalf("pre-handle callback paths: %v", paths)
	}
}
