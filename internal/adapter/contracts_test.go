package adapter

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Invoke the shipped adapter seams with literal Kit-shaped inputs. No Kit
// compilation is needed to decide whether SKGO accepts those inputs.
func runAdapterScript(t *testing.T, module, program string, env []string, args ...string) {
	t.Helper()
	web, err := filepath.Abs("../../example/web")
	if err != nil {
		t.Fatal(err)
	}
	path, err := filepath.Abs(module)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	argv := append([]string{"exec", "node", "--input-type=module", "-e", program, path}, args...)
	command := exec.CommandContext(ctx, filepath.Join(web, "node_modules/.bin/vp"), argv...)
	command.Dir = web
	command.Env = append(os.Environ(), env...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("adapter contract: %v\n%s", err, output)
	}
}

func writeAdapterFixture(t *testing.T, path, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestAdapterRejectsUnsupportedConfigAndMismatchedRegistrations(t *testing.T) {
	t.Parallel()
	const program = `import assert from 'node:assert/strict';
import {mkdirSync,writeFileSync} from 'node:fs';
import {join} from 'node:path';
import {pathToFileURL} from 'node:url';
const {checkRouteConfigs,checkRemoteHashes,checkRemoteIds,checkServerLoads}=await import(pathToFileURL(process.argv[1]));
checkRouteConfigs([{id:'/'},{id:'/empty',config:{}},{id:'/null',config:null}]);
assert.throws(()=>checkRouteConfigs([{id:'/catalog',config:{region:'antarctica'}}]),/route \/catalog exports unsupported adapter config \(region\).*Remove config/);
const manifest={_:{remotes:{'3215r6':()=>{throw new Error('must not execute Kit modules')}}}};
assert.deepEqual([...checkRemoteHashes(manifest,['3215r6/declared','3215r6/unused'])],['3215r6']);
assert.throws(()=>checkRemoteHashes({},[]),/no ` + "`remotes`" + ` map/);
assert.throws(()=>checkRemoteHashes(manifest,[]),/compiled but not generated: 3215r6/);
assert.throws(()=>checkRemoteHashes({_:{remotes:{}}},['3215r6/declared']),/generated but not compiled: 3215r6/);
checkServerLoads(['','src/routes/+page.server.ts','src/routes/+page.server.ts','src/routes/a/+page.server.js'],['src/routes/+page.server.ts'],['src/routes/a/+page.server.js']);
assert.throws(()=>checkServerLoads([],['src/routes/+page.server.ts'],[]),/generated but not compiled: src\/routes\/\+page.server.ts/);
assert.throws(()=>checkServerLoads(['src/routes/a/+page.server.js'],[],[]),/compiled but not generated: src\/routes\/a\/\+page.server.js/);
const client=process.argv[2];mkdirSync(join(client,'nested'),{recursive:true});
writeFileSync(join(client,'nested','page.js'),"export const declared=__remote.query('3215r6/declared');const unrelated='otherhash/manual';");
writeFileSync(join(client,'ignored.css'),"'3215r6/notJavascript'");
checkRemoteIds(client,['3215r6/declared','3215r6/unused'],new Set(['3215r6']));
for(const quote of ["'",'"',String.fromCharCode(96)]) {
 writeFileSync(join(client,'nested','page.js'),'export const manual=__remote.query('+quote+'3215r6/manual'+quote+');');
 assert.throws(()=>checkRemoteIds(client,['3215r6/declared'],new Set(['3215r6'])),error=>error.message.includes('3215r6/manual')&&error.message.includes('page.js')&&error.message.includes('would 404'));
}
`
	runAdapterScript(t, "skgo-adapter.js", program, nil, t.TempDir())
}

func TestPrerenderTransformsDispatchGoCallbacksInBothLanguages(t *testing.T) {
	t.Parallel()
	const program = `import assert from 'node:assert/strict';
import {mkdirSync,writeFileSync,readFileSync} from 'node:fs';
import {join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {createRequire} from 'node:module';
const {prerenderModules}=await import(pathToFileURL(process.argv[1]));
const app=createRequire(join(process.cwd(),'package.json'));
const {transformSync}=await import(pathToFileURL(app.resolve('vite/rolldown/experimental')));
const root=process.argv[2];mkdirSync(join(root,'src'),{recursive:true});
const data=code=>'data:text/javascript,'+encodeURIComponent(code);
const kit=data('export const prerender=(...args)=>({call:args.find(a=>typeof a==="function"),inputs:args.at(-1)?.inputs});export const getRequestEvent=()=>globalThis.fixtureEvent;export const error=()=>{throw new Error("unexpected load error")};export const redirect=error;export class HandledHttpError extends Error{}');
const helper=data('export const routeEntries=module=>({module,entries:[{slug:"literal-atlas"}]});export const remoteInputs=(module,name)=>({module,name,inputs:["literal-atlas"]});');
globalThis.fixtureEvent={platform:{skgoPrerenderRemote:(module,name,arg)=>({module,name,arg})}};
for(const extension of ['ts','js']) {
 const module='src/fixture.remote.'+extension;
 const type=extension==='ts'?': string':'';
 const original='// Code generated by skgo. DO NOT EDIT.\nimport {prerender} from "$app/server";export const empty=prerender(()=>{throw new Error("stub executed")});export const skgoRemoteInputs=prerender("unchecked",(arg'+type+')=>{throw new Error("stub executed")});';
 writeFileSync(join(root,module),original);
 const registry={remotes:[],loads:[],actions:[],endpoints:{},build:{[module]:{remotes:[{name:'empty',argument:false,inputs:false},{name:'skgoRemoteInputs',argument:true,inputs:true}]}}};
 const plugin=prerenderModules();plugin.configResolved({root});
 const write=()=>writeFileSync(join(root,'skgo.remotes.json'),JSON.stringify(registry));
 write();plugin.buildStart();
 const source=plugin.load(join(root,module)).replaceAll("'$app/server'",JSON.stringify(kit)).replaceAll('"$app/server"',JSON.stringify(kit)).replaceAll("'@skgo/sveltekit-adapter/prerender'",JSON.stringify(helper));
 const compiled=transformSync(module,source,{lang:extension}).code;
 const transformed=await import(data(compiled));
 assert.deepEqual(await transformed.empty.call(),{module,name:'empty',arg:undefined});
 assert.deepEqual(await transformed.skgoRemoteInputs.call('literal-atlas'),{module,name:'skgoRemoteInputs',arg:'literal-atlas'});
 assert.deepEqual(await transformed.skgoRemoteInputs.inputs(),{module,name:'skgoRemoteInputs',inputs:['literal-atlas']});
 assert.equal(readFileSync(join(root,module),'utf8'),original);
 const loadModule='src/routes/+page.server.'+extension;mkdirSync(join(root,'src/routes'),{recursive:true});
 writeFileSync(join(root,loadModule),'// Code generated by skgo. DO NOT EDIT.\nexport const load=async(event)=>{throw new Error("Go load stub executed")};');
 registry.loads=[loadModule];registry.prerender={root:'.',package:'./prerender'};registry.build={[loadModule]:{source:'src/routes/page.server.go'}};write();plugin.buildStart();
 const loadSource=plugin.load(join(root,loadModule)).replaceAll("'@sveltejs/kit'",JSON.stringify(kit)).replaceAll("'@sveltejs/kit/internal'",JSON.stringify(kit));
 const loaded=await import(data(loadSource));let loadCalls=0;
 const event={platform:{skgoPrerenderLoad:async(module,source,received)=>{
  loadCalls++;assert.equal(module,'src/routes/+page.server.'+extension);assert.equal(source,'src/routes/page.server.go');assert.equal(received,event);
  return {data:{receipt:'literal Go load 541'}};
 }}};
 assert.deepEqual(await loaded.load(event),{receipt:'literal Go load 541'});assert.equal(loadCalls,1);
 const entries='src/+page.server.'+extension;
 writeFileSync(join(root,entries),'// Code generated by skgo. DO NOT EDIT.\nexport const entries=()=>{throw new Error("stub executed")};');
 registry.loads=[];registry.build={[entries]:{entries:true}};write();plugin.buildStart();
 const entrySource=plugin.load(join(root,entries)).replaceAll("'@skgo/sveltekit-adapter/prerender'",JSON.stringify(helper));
 const entry=await import(data(entrySource));
 assert.deepEqual(await entry.entries(),{module:entries,entries:[{slug:'literal-atlas'}]});
 registry.build[entries].entries=false;write();assert.throws(()=>plugin.buildStart(),/malformed build declaration/);
}
`
	runAdapterScript(t, "skgo-adapter/prerender-modules.js", program, nil, t.TempDir())
}
