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

// A Darwin group containing only unreaped zombies rejects even SIGKILL with
// EPERM. Hold a real Go child's Wait, independently of the Node-owned leader,
// so cleanup encounters that state after the leader's close event as well.
func TestPrerenderFailureWaitsForNativeExitedGroupReap(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFixtureFile(t, filepath.Join(root, "skgo.remotes.json"), `{"prerender":{"root":".","package":"."}}`)
	const helper = `package main
import("os";"os/exec";"path/filepath";"strconv";"syscall";"time")
func main(){
 role,root:=os.Args[1],os.Args[2]
 if role=="leaf" {time.Sleep(50*time.Millisecond);return}
 exe,_:=os.Executable()
 if role=="leader" {
  cmd:=exec.Command(exe,"reaper",root,strconv.Itoa(os.Getpid()))
  cmd.SysProcAttr=&syscall.SysProcAttr{Setpgid:true}
  if err:=cmd.Start();err!=nil{panic(err)}
  deadline:=time.Now().Add(3*time.Second)
  for time.Now().Before(deadline){if _,err:=os.Stat(filepath.Join(root,"ready"));err==nil{return};time.Sleep(time.Millisecond)}
  panic("reaper did not start")
 }
 pgid,_:=strconv.Atoi(os.Args[3])
 child:=exec.Command(exe,"leaf",root)
 child.SysProcAttr=&syscall.SysProcAttr{Setpgid:true,Pgid:pgid}
 if err:=child.Start();err!=nil{panic(err)}
 receipt:=strconv.Itoa(child.Process.Pid)+"\n"+strconv.Itoa(os.Getpid())
 if err:=os.WriteFile(filepath.Join(root,"ready"),[]byte(receipt),0600);err!=nil{panic(err)}
 deadline:=time.Now().Add(5*time.Second)
 for time.Now().Before(deadline){if _,err:=os.Stat(filepath.Join(root,"reap"));err==nil{break};time.Sleep(time.Millisecond)}
 // Wait also reaps a child terminated by the owner's group SIGTERM.
 reaped:="reaped"
 if err:=child.Wait();err!=nil{
  if _,ok:=err.(*exec.ExitError);!ok{panic(err)}
  reaped+=": "+err.Error()
 }
 if err:=os.WriteFile(filepath.Join(root,"reaped"),[]byte(reaped),0600);err!=nil{panic(err)}
}
`
	helperPath := filepath.Join(root, "helper.go")
	writeFixtureFile(t, helperPath, helper)
	binary := filepath.Join(root, "helper")
	if output, err := exec.Command("go", "build", "-o", binary, helperPath).CombinedOutput(); err != nil {
		t.Fatalf("build real process helper: %v\n%s", err, output)
	}
	module, err := os.ReadFile("skgo-adapter/prerender.js")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "owner.js")
	writeFixtureFile(t, path, string(module)+"\nexport {startJob};\n")
	t.Cleanup(func() {
		// The reaper is fixture control outside the owned group. Its normal exit
		// is asserted below; still release it if Node fails before its finally.
		_ = os.WriteFile(filepath.Join(root, "reap"), []byte("reap"), 0o600)
	})
	const program = `import assert from 'node:assert/strict';
import {existsSync,mkdtempSync,readFileSync,writeFileSync} from 'node:fs';
import {spawnSync} from 'node:child_process';
import {join} from 'node:path';
import {pathToFileURL} from 'node:url';
const {createPrerenderOwner,startJob,installPrerenderFailureBoundary}=await import(pathToFileURL(process.argv[1]).href);
const root=process.argv[2],owner=createPrerenderOwner(root);
owner.dir=mkdtempSync(join(root,'private-'));
const job=startJob(owner,process.argv[3],['leader',root]);
const original=new Error('literal original native crawler failure');
try {
 await job.closed;
 const [leaf,reaper]=readFileSync(join(root,'ready'),'utf8').split('\n').map(Number);
 const state=spawnSync('ps',['-p',String(leaf),'-o','pid=,ppid=,pgid=,uid=,stat=,command='],{encoding:'utf8'});
 assert.equal(state.status,0,state.stderr);
 assert.match(state.stdout,/\sZ\S*\s/,'Go child must be exited and unreaped, not still running');
 assert.equal(Number(state.stdout.trim().split(/\s+/)[1]),reaper,'independent Go parent owns the reap gate');
 assert.equal(Number(state.stdout.trim().split(/\s+/)[2]),job.child.pid,'zombie belongs to the owned group');
 const probe=()=>{try{process.kill(-job.child.pid,0);return 'exists';}catch(error){return error.code;}};
 assert.equal(probe(),'EPERM');
 assert.throws(()=>process.kill(-job.child.pid,'SIGTERM'),{code:'EPERM'});
 assert.throws(()=>process.kill(-job.child.pid,'SIGKILL'),{code:'EPERM'});
 console.log('NATIVE_EXITED_GROUP:'+JSON.stringify({group:job.child.pid,probe:probe(),state:state.stdout.trim(),leaderClosed:true}));
 setTimeout(()=>writeFileSync(join(root,'reap'),'reap'),300);
 const builder={buildApp:async()=>{throw original;}};
 installPrerenderFailureBoundary({getSortedPlugins:()=>[],builder},owner);
 await assert.rejects(builder.buildApp(),(error)=>{
  console.log('REJECTION_RESOURCES:'+JSON.stringify({probe:probe(),privateDirExists:existsSync(owner.dir),reaped:existsSync(join(root,'reaped')),cleanupError:error.cleanupError?.message,cleanupErrors:error.cleanupError?.errors?.map(({message,code})=>({message,code}))}));
  assert.equal(error,original);
  assert.equal(error.message,'literal original native crawler failure');
  assert.equal(error.cleanupError,undefined,'native exited-group bookkeeping became a cleanup failure');
  assert.equal(probe(),'ESRCH','build rejected before actual group disappearance');
  assert.throws(()=>process.kill(leaf,0),{code:'ESRCH'},'build rejected before Go reaped the group member');
  assert.equal(existsSync(join(root,'reap')),true,'build rejected before the fixture released its reap gate');
  assert.equal(existsSync(owner.dir),false,'build rejected before private directory removal');
  return true;
 });
 console.log('NATIVE_EXITED_GROUP_REAPED_BEFORE_REJECTION');
} finally {
 writeFileSync(join(root,'reap'),'reap');
 const reaper=Number(readFileSync(join(root,'ready'),'utf8').split('\n')[1]);
 const deadline=Date.now()+5000;
 const fixtureControlAlive=()=>{try{process.kill(reaper,0);return true;}catch(error){if(error.code==='ESRCH')return false;throw error;}};
 while(fixtureControlAlive()&&Date.now()<deadline)await new Promise(resolve=>setTimeout(resolve,20));
 assert.equal(fixtureControlAlive(),false,'fixture Go reaper did not exit');
 assert.equal(existsSync(join(root,'reaped')),true,'fixture parent did not complete its Go Wait');
 console.log('FIXTURE_REAPER_EXITED:'+readFileSync(join(root,'reaped'),'utf8'));
 await owner.cleanup();
}
`
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "--input-type=module", "-e", program, path, root, binary)
	output, err := command.CombinedOutput()
	t.Logf("native exited-group cleanup:\n%s", output)
	if err != nil {
		t.Fatalf("native exited-group cleanup: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "NATIVE_EXITED_GROUP_REAPED_BEFORE_REJECTION") {
		t.Fatalf("native process assertions did not finish: %s", output)
	}
}
