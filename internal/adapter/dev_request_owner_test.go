package adapter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

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
			w.Header().Set("X-Skgo-Fetch-Request", base64.StdEncoding.EncodeToString([]byte(`{"url":"http://app.test/inner","method":"GET","headers":{}}`)))
			w.WriteHeader(204)
		case "/_skgo/dev/fetch-response":
			w.Header().Set("X-Skgo-Fetch", "literal-fetch-824")
			w.Header().Set("X-Skgo-Fetch-State", "final")
			io.Copy(w, r.Body)
		case "/_skgo/dev/fetch-end":
			w.WriteHeader(204)
		case "/_skgo/dev/response":
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
assert.equal(await response.text(),'literal-Kit-continuation-935');
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
