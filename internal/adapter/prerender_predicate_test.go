package adapter

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrerenderPredicateRelayBooleanErrorsAndTerminalLateReply(t *testing.T) {
	module, err := os.ReadFile("skgo-adapter/prerender.js")
	if err != nil {
		t.Fatal(err)
	}
	// Expose the actual private relay in a disposable module. Shorten only the
	// waiting budget; the real-build timeout contract exercises the 30s budget.
	source := strings.Replace(string(module), "const CALLBACK_TIMEOUT = 30_000;", "const CALLBACK_TIMEOUT = 100;", 1) + "\nexport {predicate};\n"
	path := filepath.Join(t.TempDir(), "relay.mjs")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	const program = `import {Worker,BroadcastChannel} from 'node:worker_threads';
import {pathToFileURL} from 'node:url';
import assert from 'node:assert/strict';
for (const mode of ['booleans','early','error','malformed','timeout']) {
 const channel=new BroadcastChannel('predicate-test-'+mode);process.env.SKGO_PRERENDER_PREDICATE_CHANNEL=channel.name;
 let calls=0,notice=false;
 channel.onmessage=({data})=>{
  if(data.type==='failure'){notice=true;return;}
  if(data.type!=='predicate')return;
  calls++;
  const answer=()=>{channel.postMessage(mode==='malformed'?{id:data.id,value:'wrong'}:mode==='error'?{id:data.id,error:'full literal error\nwith details'}:{id:data.id,value:calls===1});Atomics.store(data.cell,0,1);Atomics.notify(data.cell,0);};
  if(mode==='timeout')setTimeout(answer,200);else answer();
 };
 const workerCode="import {workerData,parentPort} from 'node:worker_threads'; (async()=>{const {predicate}=await import(workerData.module);const results=[];for(let i=0;i<3;i++){try{results.push(predicate('literal-request','preload',{input:{type:'font',path:'/literal.woff2',filename:'src/literal.woff2'}}));}catch(error){results.push(error.message);} if(workerData.mode==='timeout' && i===1) Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0,250); }parentPort.postMessage(results);})();";
 // Delay the worker immediately before Atomics.wait, allowing the real owner
 // reply to arrive first. The not-equal path must still read the answer.
 let workerModule=process.argv[1];
 if(mode==='early'){
  const {readFileSync,writeFileSync}=await import('node:fs');workerModule=process.argv[1]+'.early.mjs';
  writeFileSync(workerModule,readFileSync(process.argv[1],'utf8').replace('const status = Atomics.wait(predicateCell','Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0,50); const status = Atomics.wait(predicateCell'));
 }
 const worker=new Worker(workerCode,{eval:true,workerData:{module:pathToFileURL(workerModule).href,mode}});
 const result=await new Promise((resolve,reject)=>{worker.once('message',resolve);worker.once('error',reject);});
 await new Promise(resolve=>worker.once('exit',resolve));
 if(mode==='booleans'||mode==='early'){assert.deepEqual(result,[true,false,false]);assert.equal(calls,3);assert.equal(notice,false);}
 else{assert.equal(calls,1);assert.equal(notice,true);assert.equal(result[0],result[1]);assert.equal(result[1],result[2]);assert.match(result[0],mode==='timeout'?/timed out after 100ms/:mode==='malformed'?/missing or malformed/:/full literal error\nwith details/);}
 channel.close();
}
console.log('relay true false full-error answer-before-wait malformed timeout late-reply passed');`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "--input-type=module", "-e", program, path)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("relay: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "late-reply passed") {
		t.Fatalf("relay assertions did not finish: %s", output)
	}
}
