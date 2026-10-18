package adapter

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Go's real service handlers have their own tests. This fixture isolates the
// adapter's encoding, decoding and error propagation from that implementation.
func TestPrerenderCallbacksPreserveGoInputsEntriesAndRemoteResults(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	deps, err := filepath.Abs("../../example/web/node_modules")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(deps, filepath.Join(root, "node_modules")); err != nil {
		t.Fatal(err)
	}
	writeAdapterFixture(t, filepath.Join(root, "package.json"), `{"type":"module"}`)
	writeAdapterFixture(t, filepath.Join(root, ".svelte-kit/output/server/entries/hooks.universal.js"), `class Money{constructor(cents){this.cents=cents}label(){return this.cents+" cents"}};export const transport={Money:{encode:value=>value instanceof Money&&value.cents,decode:value=>new Money(value)}};`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer literal-callback-secret" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("callback request: %s %s %v", r.Method, r.URL, r.Header)
			http.Error(w, "wrong callback transport", 400)
			return
		}
		var body struct {
			Module, Name, Payload, URL, Handle string
			Headers                            map[string][]string
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/entries":
			if body.Module == "src/routes/catalog/+page.server.ts" {
				io.WriteString(w, `{"entries":[{"slug":"atlas"},{"slug":"beacon","edition":"v2"}]}`)
			} else if body.Module == "src/routes/broken/+page.server.ts" {
				http.Error(w, "literal producer failure 721", 500)
			} else {
				t.Errorf("entries module = %q", body.Module)
				w.WriteHeader(400)
			}
		case "/inputs":
			if body.Module != "src/data.remote.ts" {
				t.Errorf("inputs module = %q", body.Module)
				w.WriteHeader(400)
				return
			}
			switch body.Name {
			case "names":
				io.WriteString(w, `{"inputs":"[[1,2,1],\"atlas\",\"beacon\"]"}`)
			case "empty":
				io.WriteString(w, `{"inputs":"[[]]"}`)
			case "money":
				io.WriteString(w, `{"inputs":"[[1],[\"Money\",2],125]"}`)
			case "malformed":
				io.WriteString(w, `{"inputs":"[null]"}`)
			default:
				t.Errorf("inputs name = %q", body.Name)
				w.WriteHeader(400)
			}
		case "/remote":
			if body.Module != "src/data.remote.ts" || body.Payload != "WyJhdGxhcyJd" || body.URL != "http://app.test/catalog?literal=51" || body.Handle != "literal-handle-513" || len(body.Headers["x-receipt"]) != 1 || body.Headers["x-receipt"][0] != "literal-742" {
				t.Errorf("remote callback = %+v", body)
				w.WriteHeader(400)
				return
			}
			switch body.Name {
			case "value":
				io.WriteString(w, `{"type":"result","data":"[{\"_\":1},\"body:atlas\"]"}`)
			case "missing":
				io.WriteString(w, `{"type":"result","data":"[{}]"}`)
			case "failure":
				io.WriteString(w, `{"type":"error","kind":"unknown","error":{"status":500,"message":"Internal Error"},"diagnostic":"literal Go failure 921"}`)
			default:
				t.Errorf("remote name = %q", body.Name)
				w.WriteHeader(400)
			}
		default:
			t.Errorf("callback path = %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	const program = `import assert from 'node:assert/strict';
import {pathToFileURL} from 'node:url';
process.chdir(process.argv[2]);
const {routeEntries,remoteInputs,remoteFunction}=await import(pathToFileURL(process.argv[1]));
assert.deepEqual(await routeEntries('src/routes/catalog/+page.server.ts'),[{slug:'atlas'},{slug:'beacon',edition:'v2'}]);
await assert.rejects(routeEntries('src/routes/broken/+page.server.ts'),/literal producer failure 721/);
assert.deepEqual(await remoteInputs('src/data.remote.ts','names'),['atlas','beacon','atlas']);
assert.deepEqual(await remoteInputs('src/data.remote.ts','empty'),[]);
const money=await remoteInputs('src/data.remote.ts','money');assert.equal(money.length,1);assert.equal(money[0].label(),'125 cents');
await assert.rejects(remoteInputs('src/data.remote.ts','malformed'),/did not decode to an array/);
const event={request:new Request('http://app.test/catalog?literal=51',{headers:{'x-receipt':'literal-742'}}),url:new URL('http://app.test/catalog?literal=51'),platform:{skgoRequestHandle:'literal-handle-513'}};
assert.equal(await remoteFunction('src/data.remote.ts','value','atlas',event),'body:atlas');
await assert.rejects(remoteFunction('src/data.remote.ts','missing','atlas',event),/omitted its value member/);
await assert.rejects(remoteFunction('src/data.remote.ts','failure','atlas',event),/literal Go failure 921/);
`
	runAdapterScript(t, "skgo-adapter/prerender.js", program, []string{"SKGO_PRERENDER_SERVICE_URL=" + server.URL, "SKGO_PRERENDER_SERVICE_SECRET=literal-callback-secret"}, root)
}
