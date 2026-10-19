package adapter

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// SKGO must leave authored transform errors to Kit's policy, but make a broken
// async callback terminal even when its caller catches the immediate error.
func TestPrerenderTransformApplicationErrorsRemainRecoverableButTransportFailsOwner(t *testing.T) {
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
	writeAdapterFixture(t, filepath.Join(root, "skgo.remotes.json"), `{"prerender":{"root":".","package":"./literal-prerender"}}`)
	writeAdapterFixture(t, filepath.Join(root, ".svelte-kit/output/server/manifest-full.js"), `export const manifest={app_dir:'_app',app_path:'_app',routes:[]};`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Authorization") != "Bearer literal-transform-secret" {
			t.Errorf("transform callback authentication: %s %v", r.Method, r.Header)
			http.Error(w, "wrong callback", 400)
			return
		}
		var input struct {
			URL, Handle, HTML string
			Response          json.RawMessage
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			http.Error(w, "wrong JSON", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/begin":
			handle := "transport"
			if strings.Contains(input.URL, "/application") {
				handle = "application"
			}
			json.NewEncoder(w).Encode(map[string]any{"handle": handle, "resolve": true, "options": map[string]bool{"transform": true}})
		case "/resolve-option":
			if input.Handle == "application" {
				if input.HTML == "RECOVERY" {
					io.WriteString(w, `{"html":"RECOVERED_HTML_276"}`)
					return
				}
				w.WriteHeader(500)
				io.WriteString(w, `{"kind":"app","error":"literal authored transform failure 276"}`)
			} else {
				io.WriteString(w, `{"html":17}`)
			}
		case "/response":
			json.NewEncoder(w).Encode(map[string]any{"response": input.Response})
		case "/end":
			io.WriteString(w, `{}`)
		default:
			t.Errorf("unexpected transform callback %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	const program = `import assert from 'node:assert/strict';
import {pathToFileURL} from 'node:url';
process.chdir(process.argv[2]);
for(const mode of ['application','transport']){
 const url=pathToFileURL(process.argv[1]);url.searchParams.set('mode',mode);
 const {createPrerenderOwner,requestHandle}=await import(url.href);
 const owner=createPrerenderOwner(process.cwd());
 owner.connection={url:process.argv[3],secret:'literal-transform-secret'};owner.publish();
 let report;const reported=new Promise(resolve=>{report=resolve});
 const onmessage=owner.channel.onmessage;
 owner.channel.onmessage=event=>{onmessage.call(owner.channel,event);if(event.data?.type==='failure')report(event.data.error);};
 const request=new Request('http://app.test/'+mode);
 const event={request,url:new URL(request.url),route:{id:null},platform:{},setHeaders(){},cookies:{set(){}}};
 try{
  const result=requestHandle(event,async(_event,options)=>{
   let caught;
   try{await options.transformPageChunk({html:'ORIGINAL',done:true});}catch(error){caught=error;}
   assert.ok(caught,'transform fixture must throw');
   if(mode==='application'){
    assert.equal(caught.message,'literal authored transform failure 276');assert.equal(caught.applicationTransform,true);
    const recovered=await options.transformPageChunk({html:'RECOVERY',done:true});assert.equal(recovered,'RECOVERED_HTML_276');
    return new Response(recovered);
   }
   assert.match(caught.message,/returned no HTML/);assert.notEqual(caught.applicationTransform,true);
   return new Response('caller caught broken transport');
  });
  if(mode==='application'){
   const response=await result;assert.equal(await response.text(),'RECOVERED_HTML_276');assert.equal(owner.failed,null);
  }else{
   // The real request may already be unwinding when owner failure closes its
   // callback environment. Its build owner must still receive terminal notice.
   result.catch(()=>{});
   let timer;try{await Promise.race([reported,new Promise((_,reject)=>{timer=setTimeout(()=>reject(new Error('broken transform did not fail its owner')),1000)})]);}finally{clearTimeout(timer)}
   await result.catch(()=>{});await owner.cleanup();assert.match(owner.failed.message,/returned no HTML/);
  }
 }finally{await owner.cleanup();}
}
`
	runAdapterScript(t, "skgo-adapter/prerender.js", program, nil, root, server.URL)
}
