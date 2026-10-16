package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNativeResponseWaitsForLogicalRequestEnd(t *testing.T) {
	for _, test := range []struct {
		method string
		status int
		body   bool
	}{{"HEAD", 200, false}, {"GET", 204, false}, {"GET", 200, true}} {
		t.Run(fmt.Sprintf("%s-%d", test.method, test.status), func(t *testing.T) {
			dir, err := os.MkdirTemp("/tmp", "skgo-empty-owner-")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			socket := filepath.Join(dir, "go.sock")
			listener, err := net.Listen("unix", socket)
			if err != nil {
				t.Fatal(err)
			}
			ending, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			finish := func() { once.Do(func() { close(release) }) }
			defer finish()
			server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/_skgo/dev/begin":
					io.WriteString(w, `{"handle":"literal-empty-owner-347","resolve":true}`)
				case "/_skgo/dev/response":
					if test.body || test.method == "HEAD" {
						w.Header().Set("Content-Length", "1")
					}
					w.WriteHeader(test.status)
					if test.body {
						io.WriteString(w, "X")
					}
				case "/_skgo/dev/end":
					close(ending)
					<-release
					io.WriteString(w, `{}`)
				default:
					http.NotFound(w, r)
				}
			})}
			go server.Serve(listener)
			defer server.Close()
			module, err := filepath.Abs("skgo-adapter/dev-bridge.js")
			if err != nil {
				t.Fatal(err)
			}
			program := fmt.Sprintf(`
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {createServer,get} from 'node:http';
import {pathToFileURL} from 'node:url';
const {setResponse}=await import(pathToFileURL(process.argv[2]).href);
let source=readFileSync(process.argv[1],'utf8');
source=source.replace(/import \{ error, redirect, fail \} from "@sveltejs\/kit";/,'const error=()=>{throw new Error("unexpected")}; const redirect=error; const fail=error;');
source=source.replace(/import \{ ValidationError \} from "@sveltejs\/kit\/internal";/,'class ValidationError extends Error {}');
source='const devBase="";\n'+source;
const {requestHandle}=await import('data:text/javascript;base64,'+Buffer.from(source).toString('base64'));
const event={request:new Request('http://app.test/empty',{method:%q}),url:new URL('http://app.test/empty'),locals:{},route:{id:'/empty'},cookies:{getAll:()=>[]},setHeaders:()=>{}};
const respond=()=>requestHandle(event,async()=>new Response(%s,{status:%d,headers:%s}));
if (%t) {
 const server=createServer(async(_req,res)=>setResponse(res,await respond()));
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const text=await new Promise((resolve,reject)=>get('http://127.0.0.1:'+server.address().port,res=>{
  let body='';res.on('data',chunk=>body+=chunk);res.on('end',()=>resolve(body));res.on('error',reject);
 }).on('error',reject));
 assert.equal(text,'X');
} else {
 const response=await respond();assert.equal(response.status,%d);
 if(event.request.method==='HEAD')assert.equal(response.headers.get('content-length'),'1');
}
process.exit(0);
`, test.method, map[bool]string{false: "null", true: "'X'"}[test.body], test.status, map[bool]string{false: "{}", true: "{'content-length':'1'}"}[test.body], test.body, test.status)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			kitNode, err := filepath.Abs("../../example/web/node_modules/@sveltejs/kit/src/exports/node/index.js")
			if err != nil {
				t.Fatal(err)
			}
			command := exec.CommandContext(ctx, "node", "--input-type=module", "-e", program, module, kitNode)
			command.Env = append(os.Environ(), "SKGO_DEV_GO_SOCKET="+socket, "SKGO_DEV_SERVICE_SECRET=literal-secret")
			done := make(chan error, 1)
			go func() {
				output, err := command.CombinedOutput()
				if err != nil {
					err = fmt.Errorf("%w: %s", err, output)
				}
				done <- err
			}()
			select {
			case <-ending:
			case err := <-done:
				t.Fatalf("response completed before end: %v", err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			select {
			case err := <-done:
				t.Fatalf("response completed while end was held: %v", err)
			case <-time.After(20 * time.Millisecond):
			}
			finish()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNativeFetchUsesSharedOwnerAcrossKitTracedEventCopies(t *testing.T) {
	dir, err := os.MkdirTemp("", "skgo-owner-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "go.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	reached := make(chan string, 1)
	uploaded := make(chan []byte, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/_skgo/dev/begin":
			io.WriteString(w, `{"handle":"literal-owner-713","resolve":true}`)
		case "/_skgo/dev/fetch-begin":
			var input struct {
				Handle string `json:"handle"`
			}
			json.NewDecoder(r.Body).Decode(&input)
			reached <- input.Handle
			if input.Handle != "literal-owner-713" {
				http.Error(w, "missing original request owner", 409)
				return
			}
			w.Header().Set("X-Skgo-Fetch", "literal-fetch-824")
			w.Header().Set("X-Skgo-Fetch-State", "next")
			io.WriteString(w, `{"url":"http://app.test/inner","method":"GET","headers":{}}`)
		case "/_skgo/dev/fetch-response":
			body, _ := io.ReadAll(r.Body)
			uploaded <- body
			w.WriteHeader(204)
		case "/_skgo/dev/fetch-next":
			w.Header().Set("X-Skgo-Fetch", "literal-fetch-824")
			w.Header().Set("X-Skgo-Fetch-State", "final")
			w.Write(<-uploaded)
		case "/_skgo/dev/fetch-end":
			w.WriteHeader(204)
		case "/_skgo/dev/response":
			_ = http.NewResponseController(w).EnableFullDuplex()
			w.Header().Set("X-Skgo-Middleware", "literal-app-header-846")
			w.Header().Set("Connection", "keep-alive")
			w.(http.Flusher).Flush()
			io.Copy(w, r.Body)
		case "/_skgo/dev/end":
			io.WriteString(w, `{}`)
		default:
			http.Error(w, "unknown callback", 404)
		}
	})}
	go server.Serve(listener)
	defer server.Close()
	module, err := filepath.Abs("skgo-adapter/dev-bridge.js")
	if err != nil {
		t.Fatal(err)
	}
	const program = `
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {createServer,get} from 'node:http';
let source=readFileSync(process.argv[1],'utf8');
source=source.replace(/import \{ error, redirect, fail \} from "@sveltejs\/kit";/,'const error=(status,body)=>{throw Object.assign(new Error("http error"),{status,body})}; const redirect=()=>{throw new Error("unexpected redirect")}; const fail=()=>{};');
source=source.replace(/import \{ ValidationError \} from "@sveltejs\/kit\/internal";/,'class ValidationError extends Error {}');
source='const devBase="";\n'+source;
const {requestHandle,requestFetch}=await import('data:text/javascript;base64,'+Buffer.from(source).toString('base64'));
const original={request:new Request('http://app.test/nested'),url:new URL('http://app.test/nested'),locals:{},route:{id:'/nested'},params:{},cookies:{getAll:()=>[]},setHeaders:()=>{}};
const traced={...original};
const response=await requestHandle(traced,async()=>{
 const fetched=await requestFetch({event:original,request:new Request('http://app.test/inner'),fetch:async(request)=>{
  assert.equal(request.url,'http://app.test/inner');return new Response('literal-Kit-continuation-935');
 }});
 return new Response(await fetched.text());
});
const text=await response.text();
assert.equal(text,'literal-Kit-continuation-935');
assert.equal(response.headers.get('x-skgo-middleware'),'literal-app-header-846');
// Kit can buffer a fetched body before returning the page with its own length.
// Reframe the real decoded upstream response and let Node parse the HTTP bytes.
const server=createServer((_request,result)=>{
 for(const [name,value]of response.headers)result.setHeader(name,value);
 result.setHeader('content-length',Buffer.byteLength(text));result.end(text);
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
try {
 const actual=await new Promise((resolve,reject)=>get('http://127.0.0.1:'+server.address().port,result=>{
  let body='';result.on('data',chunk=>body+=chunk);result.on('end',()=>resolve(body));result.on('error',reject);
 }).on('error',reject));
 assert.equal(actual,'literal-Kit-continuation-935');
}finally{await new Promise(resolve=>server.close(resolve));}
`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "--input-type=module", "-e", program, module)
	command.Env = append(os.Environ(), "SKGO_DEV_GO_SOCKET="+socket, "SKGO_DEV_SERVICE_SECRET=literal-secret")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("shared native request owner: %v\n%s", err, output)
	}
	select {
	case handle := <-reached:
		if handle != "literal-owner-713" {
			t.Fatal(handle)
		}
	default:
		t.Fatal("native fetch never reached Go")
	}
}
