package adapter

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// A Go error hook decides what the visitor sees. It must not decide what the
// developer's terminal shows: Kit's own prerender and production SSR both print
// the thrown error and its stack before the hook's answer is used.
func TestPrerenderErrorHookStillPrintsOriginalErrorAndStack(t *testing.T) {
	t.Parallel()
	var posted struct{ Kind, Cause string }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/error" || r.Header.Get("Authorization") != "Bearer literal-diagnostic-secret" {
			t.Errorf("error callback: %s %v", r.URL, r.Header)
			http.Error(w, "wrong callback", 400)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":500,"message":"literal-visitor-message-397"}`)
	}))
	defer server.Close()
	const program = `import assert from 'node:assert/strict';
import {pathToFileURL} from 'node:url';
const {requestError}=await import(pathToFileURL(process.argv[1]));
const logged=[];console.error=(...parts)=>logged.push(parts.join(' '));
const event={platform:{skgoErrorHook:true,skgoRequestHandle:'literal-handle-397'}};
// Kit's default handler ends with String(e) for whatever the cause chain left
// over, so a thrown string or object, or a string cause, is still printed.
logged.length=0;
await requestError({event,kind:'unknown',error:'literal-string-thrown-397',issues:undefined});
assert.deepEqual(logged,['literal-string-thrown-397']);
logged.length=0;
await requestError({event,kind:'unknown',error:{toString(){return 'literal-object-thrown-397'}},issues:undefined});
assert.deepEqual(logged,['literal-object-thrown-397']);
logged.length=0;
await requestError({event,kind:'unknown',error:new Error('literal-outer-397',{cause:'literal-string-cause-397'}),issues:undefined});
assert.equal(logged.length,2);
assert.match(logged[0],/^Error: literal-outer-397\n/);
assert.equal(logged[1],'literal-string-cause-397');
logged.length=0;
function render(){throw new Error('Unknown type Money: literal-render-failure-397');}
let thrown;try{render();}catch(error){thrown=error;}
const handled=await requestError({event,kind:'unknown',error:thrown,issues:undefined});
assert.deepEqual(handled,{status:500,message:'literal-visitor-message-397'});
const output=logged.join('\n');
assert.match(output,/Error: Unknown type Money: literal-render-failure-397/);
assert.match(output,/at render /);
assert.doesNotMatch(output,/literal-diagnostic-secret|literal-visitor-message-397/);
// Without a Go hook the terminal output must not change or double up.
logged.length=0;
assert.equal(requestError({event:{platform:{}},kind:'unknown',error:thrown}),undefined);
assert.equal(logged.filter(line=>line.includes('literal-render-failure-397')).length,1);
`
	runAdapterScript(t, "skgo-adapter/prerender.js", program, []string{"SKGO_PRERENDER_SERVICE_URL=" + server.URL, "SKGO_PRERENDER_SERVICE_SECRET=literal-diagnostic-secret"})
	if posted.Kind != "unknown" || posted.Cause == "" {
		t.Fatalf("Go hook no longer receives the original cause: %+v", posted)
	}
}

// Development prints from the same Vite process Kit reports 500s from, so an
// application error hook that sanitises the page must not swallow the cause.
func TestDevelopmentErrorHookStillPrintsOriginalErrorAndStack(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "skgo-diag-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	socket := filepath.Join(dir, "go.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/_skgo/dev/error" {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{"status":500,"message":"literal-visitor-message-397"}`)
	})}
	go server.Serve(listener)
	defer server.Close()
	module, err := filepath.Abs("skgo-adapter/dev-bridge.js")
	if err != nil {
		t.Fatal(err)
	}
	const program = `import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {pathToFileURL} from 'node:url';
let source=readFileSync(process.argv[1],'utf8');
source=source.replace(/import \{ error, redirect, fail \} from "@sveltejs\/kit";/,'const error=()=>{throw new Error("unexpected")}; const redirect=error; const fail=error;');
source=source.replace(/import \{ ValidationError \} from "@sveltejs\/kit\/internal";/,'class ValidationError extends Error {}');
source='import {bridgeFetch,incomingResponse} from '+JSON.stringify(pathToFileURL(process.argv[1].replace(/dev-bridge\.js$/,'fetch-bridge.js')).href)+';\nconst devBase=""; const devAppDir="_app";\n'+source;
const {handleGoError}=await import('data:text/javascript;base64,'+Buffer.from(source).toString('base64'));
const logged=[];console.error=(...parts)=>logged.push(parts.join(' '));
const event={locals:{},platform:{skgoRequestHandle:'literal-handle-397'},request:new Request('http://app.test/pricing'),url:new URL('http://app.test/pricing'),route:{id:'/pricing'}};
// Kit's default handler ends with String(e) for whatever the cause chain left
// over, so a thrown string or object, or a string cause, is still printed.
logged.length=0;
await handleGoError({event,kind:'unknown',error:'literal-string-thrown-397'});
assert.deepEqual(logged,['literal-string-thrown-397']);
logged.length=0;
await handleGoError({event,kind:'unknown',error:{toString(){return 'literal-object-thrown-397'}}});
assert.deepEqual(logged,['literal-object-thrown-397']);
logged.length=0;
await handleGoError({event,kind:'unknown',error:new Error('literal-outer-397',{cause:'literal-string-cause-397'})});
assert.equal(logged.length,2);
assert.match(logged[0],/^Error: literal-outer-397\n/);
assert.equal(logged[1],'literal-string-cause-397');
logged.length=0;
function render(){throw new Error('Unknown type Money: literal-render-failure-397');}
let thrown;try{render();}catch(error){thrown=error;}
const handled=await handleGoError({event,kind:'unknown',error:thrown});
assert.deepEqual(handled,{status:500,message:'literal-visitor-message-397'});
const output=logged.join('\n');
assert.match(output,/Error: Unknown type Money: literal-render-failure-397/);
assert.match(output,/at render /);
assert.doesNotMatch(output,/literal-secret|literal-visitor-message-397/);
process.exit(0);
`
	command := exec.Command("node", "--input-type=module", "-e", program, module)
	command.Env = append(os.Environ(), "SKGO_DEV_GO_SOCKET="+socket, "SKGO_DEV_SERVICE_SECRET=literal-secret")
	done := make(chan struct{})
	var output []byte
	go func() {
		defer close(done)
		output, err = command.CombinedOutput()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		command.Process.Kill()
		<-done
		t.Fatal("development error diagnostic timed out")
	}
	if err != nil {
		t.Fatalf("development error diagnostic: %v\n%s", err, output)
	}
}
