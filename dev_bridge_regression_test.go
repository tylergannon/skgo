package skgo

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

func nativeBridgeRegressionFixture(t *testing.T, loads []*ServerLoad, actions []*PageAction, options SSROptions, hook bool) http.Handler {
	t.Helper()
	development := NewNativeDevelopment("secret")
	registry, err := NewLoads(LoadConfig{Origin: "http://app.test"}, loads...)
	if err != nil {
		t.Fatal(err)
	}
	remotes, err := NewRemotes(RemoteConfig{Origin: "http://app.test"})
	if err != nil {
		t.Fatal(err)
	}
	options.Actions, err = NewActions(actions...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = development.NewSSR(fstest.MapFS{}, Manifest{}, registry, remotes, options); err != nil {
		t.Fatal(err)
	}
	pages, err := development.NewStaticHandler(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	var app http.Handler = pages
	if hook {
		middleware := RequestMiddleware[struct{}, buildLocals](func(ctx context.Context, event RequestEvent[struct{}, buildLocals], resolve RequestResolve[struct{}, buildLocals]) (*http.Response, error) {
			if err := event.SetHeader("X-Hook", "literal-hook-509"); err != nil {
				return nil, err
			}
			if err := event.SetHeader("Server-Timing", "hook;dur=1"); err != nil {
				return nil, err
			}
			return resolve(ctx, event)
		})
		app = buildBoundary(middleware)(app)
	}
	handler, close, err := development.Handler(app)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(close)
	return handler
}

// Exercise the real adapter callbacks and real Go handlers. Only Kit's import
// bindings are supplied by this transport fixture; its header duplication rule
// is copied literally from the installed pin, and devalue is the installed one.
func runNativeBridgeRegression(t *testing.T, handler http.Handler, script string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "skgo-bridge-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "go.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler}
	go server.Serve(listener)
	defer server.Close()
	module, err := filepath.Abs("internal/adapter/skgo-adapter/dev-bridge.js")
	if err != nil {
		t.Fatal(err)
	}
	const setup = `
import assert from 'node:assert/strict';
import {readFileSync,realpathSync} from 'node:fs';
import {createRequire} from 'node:module';
import {pathToFileURL} from 'node:url';
import {createServer} from 'node:http';
const require=createRequire(realpathSync(process.cwd()+'/example/web/node_modules/@sveltejs/kit/package.json'));
const devalue=await import(pathToFileURL(require.resolve('devalue')));
globalThis.bridgeDevalue=devalue;
let source=readFileSync(process.argv[1],'utf8');
source=source.replace(/import \{ error, redirect, fail \} from "@sveltejs\/kit";/,'const error=(status,body)=>{throw Object.assign(new Error("http error"),{status,body})}; const redirect=(status,location)=>{throw Object.assign(new Error("redirect"),{status,location})}; const fail=(status,data)=>({status,data});');
source=source.replace(/import \{ ValidationError \} from "@sveltejs\/kit\/internal";/,'class ValidationError extends Error {}');
source='const devBase=""; const devalue=globalThis.bridgeDevalue; const transport={stringify:devalue.stringify,parse:devalue.parse,decoders:{}};\n'+source;
const {requestHandle,requestFetch,load,action}=await import('data:text/javascript;base64,'+Buffer.from(source).toString('base64'));
function event() {
 const applied={};
 return {request:new Request('http://app.test/page'),url:new URL('http://app.test/page'),locals:{},route:{id:'/page'},params:{},cookies:{getAll:()=>[],set:()=>{}},untrack:fn=>fn(),depends:()=>{},
 setHeaders:headers=>{for(const [name,value]of Object.entries(headers)){const key=name.toLowerCase();if(key in applied){if(key==='server-timing')applied[key]+=', '+value;else throw new Error('header_already_set: '+name);}else applied[key]=value;}},applied};
}
`
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "--input-type=module", "-e", setup+script, module)
	command.Env = append(os.Environ(), "SKGO_DEV_GO_SOCKET="+socket, "SKGO_DEV_SERVICE_SECRET=secret")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("native joined bridge: %v\n%s", err, output)
	}
}

func TestNativeBridgeActionEffectsOnceWithAndWithoutHook(t *testing.T) {
	for _, hook := range []bool{true, false} {
		t.Run(fmt.Sprint(hook), func(t *testing.T) {
			actions := []*PageAction{NewPageAction(ActionSpec{Module: "src/routes/+page.server.ts", Name: "save", Run: func(ctx context.Context) (any, error) {
				if err := EventFrom(ctx).SetHeader("X-Action", "literal-action-317"); err != nil {
					return nil, err
				}
				if err := EventFrom(ctx).SetHeader("Server-Timing", "action;dur=2"); err != nil {
					return nil, err
				}
				return map[string]any{"receipt": "literal-saved-883"}, nil
			}})}
			loads := []*ServerLoad{NewServerLoad(LoadSpec{Module: "page", Run: func(ctx context.Context) (any, error) {
				if err := EventFrom(ctx).SetHeader("Server-Timing", "load;dur=3"); err != nil {
					return nil, err
				}
				return map[string]any{"loaded": "literal-loaded-394"}, nil
			}})}
			handler := nativeBridgeRegressionFixture(t, loads, actions, SSROptions{}, hook)
			runNativeBridgeRegression(t, handler, fmt.Sprintf(`
const e=event();
const response=await requestHandle(e,async()=>{
 assert.deepEqual(await action('src/routes/+page.server.ts','save',e),{receipt:'literal-saved-883'});
 assert.deepEqual(await load('page',{...e,parent:()=>{throw new Error('unexpected parent');}}),{loaded:'literal-loaded-394'});
 return new Response('literal-final-781');
});
assert.equal(await response.text(),'literal-final-781');
assert.equal(e.applied['x-action'],'literal-action-317');
assert.equal(e.applied['x-hook'],%s);
assert.equal(e.applied['server-timing'],%q);
`, map[bool]string{true: "'literal-hook-509'", false: "undefined"}[hook], map[bool]string{true: "hook;dur=1, action;dur=2, load;dur=3", false: "action;dur=2, load;dur=3"}[hook]))
		})
	}
}

func TestNativeBridgeIndependentLoadsOverlapAndParentIsDemanded(t *testing.T) {
	leafStarted := make(chan struct{})
	loads := []*ServerLoad{
		NewServerLoad(LoadSpec{Module: "root", Run: func(ctx context.Context) (any, error) {
			select {
			case <-leafStarted:
				return map[string]any{"root": "literal-root-183"}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}}),
		NewServerLoad(LoadSpec{Module: "leaf", Run: func(context.Context) (any, error) {
			close(leafStarted)
			return map[string]any{"leaf": "literal-leaf-291"}, nil
		}}),
		NewServerLoad(LoadSpec{Module: "dependent", Run: func(ctx context.Context) (any, error) {
			type Data struct {
				Parent string `json:"parent"`
			}
			first, err := Parent[Data](ctx)
			if err != nil {
				return nil, err
			}
			second, err := Parent[Data](ctx)
			if err != nil {
				return nil, err
			}
			if first.Parent != "literal-parent-517" || second.Parent != "literal-parent-517" {
				return nil, fmt.Errorf("wrong parent: %+v %+v", first, second)
			}
			return map[string]any{"receipt": "literal-dependent-619"}, nil
		}}),
	}
	handler := nativeBridgeRegressionFixture(t, loads, nil, SSROptions{}, false)
	runNativeBridgeRegression(t, handler, `
const e=event();
let parentCalls=0;
const response=await requestHandle(e,async()=>{
 const root=load('root',{...e,parent:()=>{throw new Error('unexpected root parent')}});
 const leaf=load('leaf',{...e,parent:()=>{parentCalls++;return root;}});
 assert.deepEqual(await Promise.all([root,leaf]),[{root:'literal-root-183'},{leaf:'literal-leaf-291'}]);
 assert.equal(parentCalls,0);
 const dependent=await load('dependent',{...e,parent:async()=>{parentCalls++;return {parent:'literal-parent-517'};}});
 assert.deepEqual(dependent,{receipt:'literal-dependent-619'});
 assert.equal(parentCalls,1);
 return new Response('literal-parent-final-809');
});
assert.equal(await response.text(),'literal-parent-final-809');
`)
}

func TestNativeBridgeFetchBodyMetadataAndUnreadCancellation(t *testing.T) {
	options := SSROptions{HandleFetch: func(ctx context.Context, request *http.Request, next Fetch) (*http.Response, error) {
		if request.URL.Path == "/substitute" {
			return &http.Response{StatusCode: 202, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("literal-substitute-421"))}, nil
		}
		if request.Method == "POST" {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			if string(body) != strings.Repeat("body-483", 4096) {
				return nil, fmt.Errorf("unexpected body length %d", len(body))
			}
			request.Body = io.NopCloser(strings.NewReader(string(body)))
			request.Header.Set("X-Go-Before", "literal-before-519")
		}
		result, err := next(request)
		if err == nil {
			result.Header.Set("X-Go-After", "literal-after-819")
		}
		return result, err
	}}
	handler := nativeBridgeRegressionFixture(t, nil, nil, options, false)
	runNativeBridgeRegression(t, handler, `
const server=createServer((request,response)=>{
 if(request.url==='/redirect'){response.writeHead(302,{location:'/landed'});response.end();}
 else {response.end('literal-network-731');}
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
try {
 const e=event();let observeCancel;const cancelled=new Promise(resolve=>observeCancel=resolve);
 const page=await requestHandle(e,async()=>{
  const posted=await requestFetch({event:e,request:new Request('http://app.test/post',{method:'POST',body:'body-483'.repeat(4096)}),fetch:async(request)=>{
   assert.equal(await request.text(),'body-483'.repeat(4096));assert.equal(request.headers.get('x-go-before'),'literal-before-519');return new Response('literal-posted-283');
  }});
  assert.equal(await posted.text(),'literal-posted-283');assert.equal(posted.headers.get('x-go-after'),'literal-after-819');
  const origin='http://127.0.0.1:'+server.address().port;
  const followed=await requestFetch({event:e,request:new Request(origin+'/redirect'),fetch});
  assert.equal(followed.url,origin+'/landed');assert.equal(followed.redirected,true);assert.equal(followed.type,'basic');
  const cloned=followed.clone();assert.equal(cloned.url,origin+'/landed');assert.equal(cloned.redirected,true);
  assert.equal(await followed.text(),'literal-network-731');assert.equal(await cloned.text(),'literal-network-731');
  const substitute=await requestFetch({event:e,request:new Request('http://app.test/substitute'),fetch:()=>{throw new Error('replacement used continuation')}});
  assert.equal(substitute.url,'');assert.equal(substitute.redirected,false);assert.equal(await substitute.text(),'literal-substitute-421');
  // The caller intentionally only inspects status, a valid SSR fetch consumer.
  const unread=await requestFetch({event:e,request:new Request('http://app.test/unread'),fetch:async()=>new Response(new ReadableStream({start(c){c.enqueue(new TextEncoder().encode('literal-unread-362'));},cancel(){observeCancel();}}))});
  assert.equal(unread.ok,true);
  return new Response('literal-finished-927');
 });
 assert.equal(await page.text(),'literal-finished-927');
 let cancelTimer;try{await Promise.race([cancelled,new Promise((_,reject)=>cancelTimer=setTimeout(()=>reject(new Error('unread fetch was not cancelled')),1000))]);}finally{clearTimeout(cancelTimer);}
}finally{await new Promise(resolve=>server.close(resolve));}
`)
}

func TestNativeBridgeConcurrentFetchContinuationsKeepTheirReplies(t *testing.T) {
	options := SSROptions{HandleFetch: func(ctx context.Context, request *http.Request, next Fetch) (*http.Response, error) {
		type result struct {
			response *http.Response
			err      error
		}
		results := make(chan result, 2)
		for _, path := range []string{"/first", "/second"} {
			go func(path string) {
				child := request.Clone(ctx)
				child.URL.Path = path
				response, err := next(child)
				if err == nil {
					body, readErr := io.ReadAll(response.Body)
					response.Body.Close()
					if readErr != nil {
						err = readErr
					} else if string(body) != "literal-"+path[1:]+"-437" {
						err = fmt.Errorf("continuation %s got %q", path, body)
					}
					response.Body = io.NopCloser(strings.NewReader(string(body)))
				}
				results <- result{response, err}
			}(path)
		}
		var chosen *http.Response
		for range 2 {
			result := <-results
			if result.err != nil {
				return nil, result.err
			}
			if result.response.Header.Get("X-Continuation") == "/second" {
				chosen = result.response
			}
		}
		if chosen == nil {
			return nil, fmt.Errorf("second continuation disappeared")
		}
		return chosen, nil
	}}
	handler := nativeBridgeRegressionFixture(t, nil, nil, options, false)
	runNativeBridgeRegression(t, handler, `
const e=event();const visited=[];
const page=await requestHandle(e,async()=>{
 const response=await requestFetch({event:e,request:new Request('http://app.test/start'),fetch:async(request)=>{
  const path=new URL(request.url).pathname;visited.push(path);
  return new Response('literal-'+path.slice(1)+'-437',{headers:{'X-Continuation':path}});
 }});
 assert.equal(await response.text(),'literal-second-437');
 assert.equal(response.headers.get('x-continuation'),'/second');
 return new Response('literal-concurrent-903');
});
assert.equal(await page.text(),'literal-concurrent-903');
assert.deepEqual(visited.sort(),['/first','/second']);
`)
}

func TestNativeBridgeParentFailurePreservesKitErrorAndCancelsGo(t *testing.T) {
	handler := nativeBridgeRegressionFixture(t, []*ServerLoad{NewServerLoad(LoadSpec{Module: "dependent", Run: func(ctx context.Context) (any, error) { return Parent[map[string]any](ctx) }})}, nil, SSROptions{}, false)
	runNativeBridgeRegression(t, handler, `
const e=event();const parentError=new Error('literal-parent-failure-809');
await assert.rejects(requestHandle(e,async()=>{
 await load('dependent',{...e,parent:async()=>{throw parentError;}});
 return new Response('unexpected');
}),error=>error===parentError);
`)
}

func TestNativeBridgeConcurrentLoadEffectsOnlyApplyOnce(t *testing.T) {
	bothStarted := make(chan struct{})
	var started atomic.Int32
	var loads []*ServerLoad
	for _, module := range []string{"first", "second"} {
		loads = append(loads, NewServerLoad(LoadSpec{Module: module, Run: func(ctx context.Context) (any, error) {
			if err := EventFrom(ctx).SetHeader("X-"+module, "literal-"+module+"-127"); err != nil {
				return nil, err
			}
			if err := EventFrom(ctx).SetHeader("Server-Timing", module+";dur=4"); err != nil {
				return nil, err
			}
			if started.Add(1) == 2 {
				close(bothStarted)
			}
			select {
			case <-bothStarted:
				return map[string]any{"module": module}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}}))
	}
	handler := nativeBridgeRegressionFixture(t, loads, nil, SSROptions{}, true)
	runNativeBridgeRegression(t, handler, `
const e=event();
const page=await requestHandle(e,async()=>{
 const independent={...e,parent:()=>{throw new Error('unexpected parent')}};
 assert.deepEqual(await Promise.all([load('first',independent),load('second',independent)]),[{module:'first'},{module:'second'}]);
 return new Response('literal-effects-final-328');
});
assert.equal(await page.text(),'literal-effects-final-328');
assert.equal(e.applied['x-first'],'literal-first-127');
assert.equal(e.applied['x-second'],'literal-second-127');
assert.deepEqual(e.applied['server-timing'].split(', ').sort(),['first;dur=4','hook;dur=1','second;dur=4']);
`)
}
