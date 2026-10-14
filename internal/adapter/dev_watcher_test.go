package adapter

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDevTemplateWatcherReportsAndRecoversFromPartialSave(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	module, err := filepath.Abs("skgo-adapter/env.js")
	if err != nil {
		t.Fatal(err)
	}
	app, err := filepath.Abs("../../example/web")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	const source = `
import { EventEmitter } from 'node:events';
import { readFileSync, writeFileSync, existsSync, unlinkSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import assert from 'node:assert/strict';
const { gojaDevUnchangedFiles } = await import(pathToFileURL(process.argv[1]));
const root = process.argv[2];
const kit = await import(pathToFileURL(process.argv[3]));
const file = join(root, 'app.html');
const watcher = new EventEmitter();
const errors = [], messages = [], rendered = [];
const server = { watcher, config: { root, logger: { error: e => errors.push(e) } }, ws: { send: m => messages.push(m) } };
gojaDevUnchangedFiles().configureServer(server);
watcher.on('all', (_, changed) => {
 if (changed !== file) return;
 if (!existsSync(file)) kit.app_template_missing({ file: 'app.html' });
 const text = readFileSync(file, 'utf8');
 for (const tag of ['%sveltekit.head%', '%sveltekit.body%']) {
  if (!text.includes(tag)) kit.app_template_tag_missing({ file: 'app.html', tag });
 }
 rendered.push(text);
});
const save = text => {
 writeFileSync(file, text);
 watcher.emit('change', file);
 watcher.emit('all', 'change', file);
};
save('');
assert.equal(messages.length, 1);
assert.equal(messages[0].type, 'error');
assert.match(messages[0].err.message, /^app_template_tag_missing\napp.html is missing \x60%sveltekit.head%\x60\n/);
assert.equal(errors.length, 1);
assert.deepEqual(rendered, []);
save('%sveltekit.head%');
assert.equal(messages.length, 2);
assert.match(messages[1].err.message, /^app_template_tag_missing\napp.html is missing \x60%sveltekit.body%\x60\n/);
unlinkSync(file);
watcher.emit('all', 'unlink', file);
assert.match(messages[2].err.message, /^app_template_missing\napp.html does not exist\n/);
save('%sveltekit.head% restored %sveltekit.body%');
assert.deepEqual(rendered, ['%sveltekit.head% restored %sveltekit.body%']);
// The guard must not swallow other files, codes, tags, names or exceptions.
const capture = fn => { try { fn(); } catch (error) { return error; } throw new Error('helper did not throw'); };
const wrongName = capture(() => kit.app_template_tag_missing({file: 'app.html', tag: '%sveltekit.head%'}));
wrongName.name = 'Error';
const unrelated = [
 new Error('unrelated watcher failure'), wrongName,
 capture(() => kit.app_template_tag_missing({file: 'other.html', tag: '%sveltekit.head%'})),
 capture(() => kit.app_template_tag_missing({file: 'app.html', tag: '%sveltekit.assets%'})),
 capture(() => kit.app_template_missing({file: 'other.html'})),
 capture(() => kit.config_alias_key_invalid({key: 'app.html'})),
];
let thrown;
watcher.on('all', () => { throw thrown; });
for (const [i, error] of unrelated.entries()) {
 thrown = error;
 assert.throws(() => save('%sveltekit.head% next ' + i + ' %sveltekit.body%'), e => e === error);
}

`
	cmd := exec.Command(node, "--input-type=module", "-e", source, module, root, filepath.Join(app, "node_modules/@sveltejs/kit/src/messages/build-errors.js"))
	cmd.Dir = app
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("dev template watcher: %v\n%s", err, out)
	}
}

func TestDevWatcherResolvedConfiguration(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	module, err := filepath.Abs("skgo-adapter/env.js")
	if err != nil {
		t.Fatal(err)
	}
	app, err := filepath.Abs("../../example/web")
	if err != nil {
		t.Fatal(err)
	}
	const source = `
import { pathToFileURL } from 'node:url';
import assert from 'node:assert/strict';
const { gojaDevUnchangedFiles } = await import(pathToFileURL(process.argv[1]));
const { resolveConfig } = await import(pathToFileURL(process.argv[2] + '/node_modules/vite/dist/vite/node/index.js'));
for (const setting of [undefined, false, true, { stabilityThreshold: 375, pollInterval: 30 }]) {
 const config = await resolveConfig({ root: process.argv[2], configFile: false, plugins: [gojaDevUnchangedFiles()],
  server: { watch: { ignored: ['**/authored-ignore/**'], ...(setting === undefined ? {} : { awaitWriteFinish: setting }) } }
 }, 'serve');
 assert.deepEqual(config.server.watch.awaitWriteFinish, setting === undefined ? { stabilityThreshold: 200, pollInterval: 25 } : setting);
 assert.deepEqual(config.server.watch.ignored, ['**/authored-ignore/**', process.argv[2] + '/build', process.argv[2] + '/build/**']);
}
`
	cmd := exec.Command(node, "--input-type=module", "-e", source, module, app)
	cmd.Dir = app
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("resolved dev watcher: %v\n%s", err, out)
	}
}

func TestDevTransformBeforeSettledChangePreservesDelivery(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	module, err := filepath.Abs("skgo-adapter/env.js")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	const source = `
import { EventEmitter } from 'node:events';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import assert from 'node:assert/strict';
const { gojaDevUnchangedFiles } = await import(pathToFileURL(process.argv[1]));
const root = process.argv[2], file = join(root, 'probe.ts');
const plugin = gojaDevUnchangedFiles(), watcher = new EventEmitter();
plugin.configureServer({ watcher, config: { root } });
const changed = [], all = [];
watcher.on('change', () => changed.push(readFileSync(file, 'utf8')));
watcher.on('all', () => all.push(readFileSync(file, 'utf8')));
const original = 'export const probe = 1;\n', edited = 'export const probe = 2;\n';
const emit = () => { watcher.emit('change', file); watcher.emit('all', 'change', file); };
writeFileSync(file, original);
plugin.transform.handler(original, file);
writeFileSync(file, edited);
emit();
plugin.transform.handler(edited, file);
writeFileSync(file, original);
// Vite may transform a requested module before awaitWriteFinish delivers it.
plugin.transform.handler(original, file);
emit();
assert.deepEqual(changed, [edited, original]);
assert.deepEqual(all, [edited, original]);
// Identical save events remain suppressed after the actual restore is delivered.
writeFileSync(file, original);
emit();
assert.deepEqual(changed, [edited, original]);
assert.deepEqual(all, [edited, original]);
`
	cmd := exec.Command(node, "--input-type=module", "-e", source, module, root)
	cmd.Dir, err = filepath.Abs("../../example/web")
	if err != nil {
		t.Fatal(err)
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("transform before settled change: %v\n%s", err, output)
	}
}

func TestGoSupervisorUsesViteReloadAndErrorProtocol(t *testing.T) {
	module, err := filepath.Abs("skgo-adapter/env.js")
	if err != nil {
		t.Fatal(err)
	}
	app, err := filepath.Abs("../../example/web")
	if err != nil {
		t.Fatal(err)
	}
	const source = `
import { EventEmitter } from 'node:events';
import { readFileSync, statSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import assert from 'node:assert/strict';
const { goSupervisorReload } = await import(pathToFileURL(process.argv[1]));
const root=process.argv[2], messages=[], handlers=new Map();
let middleware;
goSupervisorReload().configureServer({config:{root,base:'/nested/'},ws:{send:m=>messages.push(m),on:(e,f)=>handlers.set(e,f)},middlewares:{use:f=>middleware=f}});
const file=root+'/.svelte-kit/skgo-dev-supervisor.json';
const control=JSON.parse(readFileSync(file,'utf8'));
assert.equal(control.base,'/nested/');
assert.equal(statSync(file).mode & 0o777,0o600);
assert.equal(control.token.length,64);
assert.equal(control.clientFiles.length,1);
assert.match(control.clientFiles[0],/^\/nested\/@fs\/.+\/env\.mjs$/);
const deliver=(body,token=control.token,method='POST')=>{
 const req=Object.assign(new EventEmitter(),{url:'/__skgo_dev/supervisor',headers:{'x-skgo-dev-token':token},method,setEncoding(){}});
 const res={statusCode:200,end(){}};
 middleware(req,res,()=>assert.fail('control request fell through'));
 req.emit('data',body);req.emit('end');return res.statusCode;
};
assert.equal(deliver(JSON.stringify({diagnostic:'broken source <literal>'}),'wrong'),403);
assert.equal(messages.length,0);
assert.equal(deliver('no json'),400);
assert.equal(deliver('{}'),400);
assert.equal(deliver(JSON.stringify({diagnostic:'broken source <literal>'})),204);
assert.deepEqual(messages,[{type:'error',err:{message:'broken source <literal>',stack:'',plugin:'skgo'}}]);
const newcomer=[];
handlers.get('vite:client:connect')(undefined,{send:m=>newcomer.push(m)});
assert.deepEqual(newcomer,messages);
assert.equal(deliver(JSON.stringify({diagnostic:''})),204);
assert.deepEqual(messages[1],{type:'full-reload'});
handlers.get('vite:client:connect')(undefined,{send:()=>assert.fail('stale error after repair')});
// Exercise the installed Vite's own import analysis, including its rewritten
// env.mjs URL. A mock of the websocket protocol cannot prove client bootstrap.
const { createServer } = await import('vite');
for (const base of ['/', '/nested/']) {
 const server=await createServer({root,base,configFile:false,appType:'custom',plugins:[goSupervisorReload()],server:{middlewareMode:true,watch:null},optimizeDeps:{noDiscovery:true,include:[]}});
 try {
  const client=await server.transformRequest('/@vite/client');
  const current=JSON.parse(readFileSync(file,'utf8'));
  const dependency=client.code.match(/['"]([^'"]+\/env\.mjs)['"]/);
  assert.ok(dependency,'installed Vite client did not import env.mjs');
  assert.deepEqual(current.clientFiles,[dependency[1]],'supervisor cannot serve actual Vite client dependency');
 } finally { await server.close(); }
}
`
	cmd := exec.Command("node", "--input-type=module", "-e", source, module, t.TempDir())
	cmd.Dir = app
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("supervisor adapter: %v\n%s", err, out)
	}
}
