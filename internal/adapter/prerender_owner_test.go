package adapter

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// Reuse this test executable as an inspectable child, rather than compiling a
// fresh application just to exercise the adapter's process ownership. The
// compiler fixture enforces the actual arguments and emits the service fixture.
func adapterProcessFixture() int {
	mode := os.Getenv("SKGO_ADAPTER_PROCESS_FIXTURE")
	if mode == "descendant-child" {
		signal.Ignore(syscall.SIGTERM)
		fmt.Println("descendant ready")
		select {}
	}
	if len(os.Args) > 1 {
		if len(os.Args) != 5 || strings.Join(os.Args[1:3], " ") != "build -o" || os.Args[4] != "./literal-prerender" {
			fmt.Fprintln(os.Stderr, "unexpected compiler arguments", os.Args[1:])
			return 18
		}
		receipt, err := os.OpenFile(os.Getenv("SKGO_ADAPTER_COMPILER_RECEIPT"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			panic(err)
		}
		fmt.Fprintln(receipt, "build -o <private binary> ./literal-prerender")
		receipt.Close()
		if mode == "compiler-failure" {
			fmt.Fprintln(os.Stderr, "literal compiler failure 471")
			return 17
		}
		self, err := os.Executable()
		if err != nil {
			panic(err)
		}
		input, err := os.Open(self)
		if err != nil {
			panic(err)
		}
		defer input.Close()
		output, err := os.OpenFile(os.Args[3], os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
		if err != nil {
			panic(err)
		}
		if _, err := io.Copy(output, input); err != nil {
			panic(err)
		}
		if err := output.Close(); err != nil {
			panic(err)
		}
		return 0
	}
	control := os.NewFile(3, "owner control")
	if control == nil {
		panic("missing owner control")
	}
	defer control.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer listener.Close()
	if mode == "descendant" {
		self, err := os.Executable()
		if err != nil {
			panic(err)
		}
		child := exec.Command(self)
		child.Env = append(os.Environ(), "SKGO_ADAPTER_PROCESS_FIXTURE=descendant-child")
		child.Stderr = os.Stderr
		stdout, err := child.StdoutPipe()
		if err != nil {
			panic(err)
		}
		if err := child.Start(); err != nil {
			panic(err)
		}
		ready := make([]byte, len("descendant ready\n"))
		if _, err := io.ReadFull(stdout, ready); err != nil {
			panic(err)
		}
		// Keep the leader's inherited pipes open, as a real subprocess can.
		go io.Copy(os.Stdout, stdout)
	}
	ready := map[string]string{"url": "http://" + listener.Addr().String(), "secret": "literal-service-secret-381"}
	if mode == "invalid-readiness" {
		ready["url"] = "http://external.test:4888"
	}
	if err := json.NewEncoder(control).Encode(ready); err != nil {
		panic(err)
	}
	io.Copy(io.Discard, control)
	return 0
}

func TestPrerenderOwnerCompilesOncePublishesAndDrainsOnEveryFailure(t *testing.T) {
	t.Parallel()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.Symlink(self, filepath.Join(bin, "go")); err != nil {
		t.Fatal(err)
	}
	const program = `import assert from 'node:assert/strict';
import {mkdirSync,writeFileSync,readFileSync,existsSync} from 'node:fs';
import {join} from 'node:path';
import {pathToFileURL} from 'node:url';
const {createPrerenderOwner,installPrerenderFailureBoundary}=await import(pathToFileURL(process.argv[1]));
const fixture=process.argv[2];
const old={SKGO_PRERENDER_SERVICE_URL:'prior-url',SKGO_PRERENDER_SERVICE_SECRET:'prior-secret',SKGO_PRERENDER_PREDICATE_CHANNEL:'prior-channel'};
Object.assign(process.env,old);
for(const mode of ['normal','compiler-failure','invalid-readiness','build-failure','caught-callback','descendant']) {
 const root=join(fixture,mode);mkdirSync(root);
 writeFileSync(join(root,'skgo.remotes.json'),JSON.stringify({prerender:{root:'.',package:'./literal-prerender'}}));
 process.env.SKGO_ADAPTER_PROCESS_FIXTURE=mode;
 process.env.SKGO_ADAPTER_COMPILER_RECEIPT=join(root,'compiler-receipt');
 const owner=createPrerenderOwner(root);assert.equal(createPrerenderOwner(root),owner);
 let pid;
 try {
  const prepare=owner.prepare();assert.equal(owner.prepare(),prepare);
  if(mode==='compiler-failure'||mode==='invalid-readiness') {
   await assert.rejects(prepare,mode==='compiler-failure'?/literal compiler failure 471/:/invalid readiness/);
  } else {
   await prepare;pid=owner.service.child.pid;
   assert.equal(owner.connection.secret,'literal-service-secret-381');
   assert.equal(owner.jobs.size,1);
   owner.publish();const channel=owner.channel;owner.publish();assert.equal(owner.channel,channel);
   assert.equal(process.env.SKGO_PRERENDER_SERVICE_SECRET,'literal-service-secret-381');
   assert.equal(process.env.SKGO_PRERENDER_SERVICE_URL,owner.connection.url);
   assert.equal(process.env.SKGO_PRERENDER_PREDICATE_CHANNEL,channel.name);
   if(mode==='build-failure'||mode==='caught-callback') {
    const failure=new Error('literal build failure 741');
    const plugin={buildApp:{order:'post',handler:async()=>{if(mode==='build-failure')throw failure;owner.failed=failure;return 'application caught callback';}}};
    const config={getSortedPlugins:()=>[plugin]};
    installPrerenderFailureBoundary(config,owner);
    const wrapped=plugin.buildApp.handler;installPrerenderFailureBoundary(config,owner);assert.equal(plugin.buildApp.handler,wrapped);assert.equal(plugin.buildApp.order,'post');
    await assert.rejects(plugin.buildApp.handler(),error=>error===failure);
    assert.equal(existsSync(owner.dir),false,'build rejection must wait for cleanup');
   } else {
    await owner.cleanup();
   }
  }
  assert.equal(readFileSync(join(root,'compiler-receipt'),'utf8'),'build -o <private binary> ./literal-prerender\n');
  assert.equal(existsSync(owner.dir),false);
  assert.equal(owner.channel??null,null);
  for(const [key,value] of Object.entries(old))assert.equal(process.env[key],value);
  const service=owner.service?.child.pid;
  if(service)assert.throws(()=>process.kill(process.platform==='win32'?service:-service,0),error=>error.code==='ESRCH');
 } finally {await owner.cleanup();}
}
const empty=join(fixture,'empty');mkdirSync(empty);writeFileSync(join(empty,'skgo.remotes.json'),'{}');assert.equal(createPrerenderOwner(empty),null);
`
	runAdapterScript(t, "skgo-adapter/prerender.js", program, []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")}, t.TempDir())
}

func TestPrerenderFailureBoundaryWrapsAllWholeAppHooks(t *testing.T) {
	t.Parallel()
	const program = `import assert from 'node:assert/strict';
import {pathToFileURL} from 'node:url';
const {installPrerenderFailureBoundary}=await import(pathToFileURL(process.argv[1]));
for(const shape of ['function','object','builder']) {
 const failure=new Error('literal failure '+shape);let released,called=false,settled=false;
 const drain=new Promise(resolve=>{released=resolve;});
 const owner={failed:null,async fail(error){assert.equal(error,failure);called=true;await drain;}};
 const receiver={literal:'receiver'};const handler=async function(argument){assert.equal(this,receiver);assert.equal(argument,'literal-argument');throw failure;};
 const plugin={buildApp:shape==='object'?{order:'pre',handler}:handler};
 const config={getSortedPlugins:()=>shape==='builder'?[]:[plugin],builder:shape==='builder'?{buildApp:handler}:{}};
 installPrerenderFailureBoundary(config,owner);
 const wrapped=shape==='builder'?config.builder.buildApp:shape==='object'?plugin.buildApp.handler:plugin.buildApp;
 const result=wrapped.call(receiver,'literal-argument');result.catch(()=>{settled=true;});
 await new Promise(resolve=>setImmediate(resolve));assert.equal(called,true);assert.equal(settled,false);
 released();await assert.rejects(result,error=>error===failure);
}
`
	runAdapterScript(t, "skgo-adapter/prerender.js", program, nil)
}
