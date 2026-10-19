//go:build skgo_qualification && !windows

package adapter

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPrerenderFailedDrainRetainsOwnerAndCanRecover(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeAdapterFixture(t, filepath.Join(root, "skgo.remotes.json"), `{"prerender":{"root":".","package":"."}}`)
	module, err := os.ReadFile("skgo-adapter/prerender.js")
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the production owner and job lifecycle against a real detached
	// process; only the signal syscall and cleanup waiting budget are controlled.
	source := strings.Replace(string(module), "const CLEANUP_TIMEOUT = 5_000;", "const CLEANUP_TIMEOUT = 100;", 1) + "\nexport {startJob};\n"
	path := filepath.Join(root, "owner.js")
	writePrerenderOwnerFixture(t, path, source)
	const program = `import assert from 'node:assert/strict';
import {existsSync,mkdtempSync} from 'node:fs';
import {join} from 'node:path';
import {pathToFileURL} from 'node:url';
const {createPrerenderOwner,startJob}=await import(pathToFileURL(process.argv[1]).href);
const root=process.argv[2],owner=createPrerenderOwner(root);
owner.dir=mkdtempSync(join(root,'private-'));
const job=startJob(owner,process.execPath,['-e',"process.stdout.write('READY\\n');setInterval(()=>{},1000)"]);
await new Promise((resolve,reject)=>{job.child.stdout.once('data',resolve);job.child.once('error',reject);});
const kill=process.kill;
let denied=0;
process.kill=function(pid,signal){
 if(pid===-job.child.pid && signal!==0){denied++;throw Object.assign(new Error('literal signal denial EPERM'),{code:'EPERM'});}
 return kill.call(process,pid,signal);
};
const original=new Error('literal original crawler failure');
try{
 await assert.rejects(owner.fail(original),(error)=>{
  assert.equal(error,original);
  assert.match(error.message,/literal original crawler failure/);
  assert.match(error.message,/cleanup failed.*literal signal denial EPERM/);
  assert.equal(error.cleanupError,owner.cleanupFailure);
  return true;
 });
 assert.ok(denied>=2,'both TERM and KILL were denied');
 assert.equal(createPrerenderOwner(root),owner,'failed owner was discarded');
 assert.equal(owner.cleanupPromise,null,'failed cleanup could not be retried');
 assert.ok(owner.jobs.has(job),'live job ownership was discarded');
 assert.ok(existsSync(owner.dir),'failed cleanup removed its private directory');
 assert.ok(process.listeners('exit').includes(owner.exitHandler),'exit fallback was discarded');
 kill.call(process,-job.child.pid,0);
 process.kill=kill;
 await owner.cleanup();
 assert.equal(owner.jobs.size,0);
 assert.equal(existsSync(owner.dir),false);
 assert.equal(process.listeners('exit').includes(owner.exitHandler),false);
 assert.throws(()=>kill.call(process,-job.child.pid,0),{code:'ESRCH'});
 assert.equal(owner.failed,original,'recovery replaced original failure');
 const replacement=createPrerenderOwner(root);
 assert.notEqual(replacement,owner,'successful cleanup retained stale owner');
 await replacement.cleanup();
 console.log('FAILED_DRAIN_OWNERSHIP_RETAINED_AND_RECOVERED');
}finally{
 process.kill=kill;
 await owner.cleanup();
}
`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "--input-type=module", "-e", program, path, root)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("failed-drain recovery: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "FAILED_DRAIN_OWNERSHIP_RETAINED_AND_RECOVERED") {
		t.Fatalf("recovery assertions did not finish: %s", output)
	}
}

func TestPrerenderInterruptedFailedDrainExitsWithOriginalSignal(t *testing.T) {
	t.Parallel()
	for _, signal := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(signal.String(), func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeAdapterFixture(t, filepath.Join(root, "skgo.remotes.json"), `{"prerender":{"root":".","package":"."}}`)
			module, err := os.ReadFile("skgo-adapter/prerender.js")
			if err != nil {
				t.Fatal(err)
			}
			source := strings.Replace(string(module), "const CLEANUP_TIMEOUT = 5_000;", "const CLEANUP_TIMEOUT = 100;", 1) + "\nexport {startJob};\n"
			path := filepath.Join(root, "owner.js")
			writePrerenderOwnerFixture(t, path, source)
			receipt := filepath.Join(root, "child-pid")
			t.Cleanup(func() {
				data, err := os.ReadFile(receipt)
				if err != nil {
					return
				}
				pid, err := strconv.Atoi(string(data))
				if err == nil && pid > 1 {
					_ = syscall.Kill(-pid, syscall.SIGKILL)
				}
			})
			const program = `import {mkdtempSync,writeFileSync} from 'node:fs';
import {join} from 'node:path';
import {pathToFileURL} from 'node:url';
const {createPrerenderOwner,startJob}=await import(pathToFileURL(process.argv[1]).href);
const root=process.argv[2],owner=createPrerenderOwner(root);
owner.dir=mkdtempSync(join(root,'private-'));
const job=startJob(owner,process.execPath,['-e',"process.stdout.write('READY\\n');setInterval(()=>{},1000)"]);
writeFileSync(join(root,'child-pid'),String(job.child.pid));
await new Promise((resolve,reject)=>{job.child.stdout.once('data',resolve);job.child.once('error',reject);});
const kill=process.kill;
process.kill=function(pid,signal){
 if(pid===-job.child.pid && signal!==0)throw Object.assign(new Error('literal interrupted drain EPERM'),{code:'EPERM'});
 return kill.call(process,pid,signal);
};
process.kill(process.pid,process.argv[3]);
`
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			signalName := "SIGINT"
			if signal == syscall.SIGTERM {
				signalName = "SIGTERM"
			}
			command := exec.CommandContext(ctx, "node", "--input-type=module", "-e", program, path, root, signalName)
			command.WaitDelay = time.Second
			output, err := command.CombinedOutput()
			t.Logf("%s failed-drain interruption:\n%s", signalName, output)
			var exited *exec.ExitError
			if ctx.Err() != nil || !errors.As(err, &exited) {
				t.Fatalf("interrupted owner did not terminate promptly: %v deadline=%v\n%s", err, ctx.Err(), output)
			}
			status, ok := exited.Sys().(syscall.WaitStatus)
			if !ok || !(status.Signaled() && status.Signal() == signal || status.Exited() && status.ExitStatus() == 128+int(signal)) {
				t.Fatalf("interrupted owner lost original signal %s: %v\n%s", signalName, err, output)
			}
			if !strings.Contains(string(output), "skgo prerender owner failure: skgo prerender build interrupted by "+signalName) || !strings.Contains(string(output), "cleanup failed:") || !strings.Contains(string(output), "literal interrupted drain EPERM") {
				t.Fatalf("interrupted cleanup lost original failure or denial diagnostic:\n%s", output)
			}
		})
	}
}
