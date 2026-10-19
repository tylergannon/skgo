package adapter

import (
	"os"
	"path/filepath"
	"testing"
)

// Kit resolves build configuration for sync too. Configuration must work before
// generation, while actual builds require valid inputs and retain cleanup.
func TestConfigurationDefersGeneratedPrerenderInputsUntilBuild(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	deps, err := filepath.Abs("../../example/web/node_modules")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(deps, filepath.Join(root, "node_modules")); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.Symlink(self, filepath.Join(bin, "go")); err != nil {
		t.Fatal(err)
	}
	const program = `import assert from 'node:assert/strict';
import {existsSync,writeFileSync,readFileSync} from 'node:fs';
import {join} from 'node:path';
import {pathToFileURL} from 'node:url';
process.chdir(process.argv[2]);
const {default:skgo}=await import(pathToFileURL(process.argv[1]));
const {createPrerenderOwner}=await import(new URL('./skgo-adapter/prerender.js',pathToFileURL(process.argv[1])));
const previous={...process.env};
const plugin=skgo().vite.plugins.pre.find(plugin=>plugin.name==='skgo-prerender-main-owner');
const failure=new Error('literal later build failure 302');
const config={command:'build',getSortedPlugins:()=>[plugin],builder:{async buildApp(){throw failure;}}};
await plugin.configResolved(config);
const manifest=join(process.cwd(),'skgo.remotes.json');
const receipt=join(process.cwd(),'compiler-receipt');
assert.equal(existsSync(manifest),false);
assert.equal(existsSync(receipt),false);
await assert.rejects(plugin.buildApp.handler(),error=>error.code==='ENOENT'&&error.path===manifest);
writeFileSync(manifest,'{');
await assert.rejects(plugin.buildApp.handler(),SyntaxError);
writeFileSync(manifest,'{}');
await plugin.buildApp.handler();
assert.equal(existsSync(receipt),false);
writeFileSync(manifest,JSON.stringify({prerender:{root:'.',package:'./literal-prerender'}}));
await plugin.buildApp.handler();
assert.equal(readFileSync(receipt,'utf8'),'build -o <private binary> ./literal-prerender\n');
const owner=createPrerenderOwner(process.cwd());
const pid=owner.service.child.pid;
assert.equal(process.env.SKGO_PRERENDER_SERVICE_SECRET,'literal-service-secret-381');
await assert.rejects(config.builder.buildApp(),error=>error===failure);
assert.equal(existsSync(owner.dir),false,'build failure must wait for cleanup');
assert.throws(()=>process.kill(process.platform==='win32'?pid:-pid,0),error=>error.code==='ESRCH');
for(const key of ['SKGO_PRERENDER_SERVICE_URL','SKGO_PRERENDER_SERVICE_SECRET','SKGO_PRERENDER_PREDICATE_CHANNEL'])assert.equal(process.env[key],previous[key]);
`
	runAdapterScript(t, "skgo-adapter.js", program, []string{
		"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
		"SKGO_ADAPTER_PROCESS_FIXTURE=normal",
		"SKGO_ADAPTER_COMPILER_RECEIPT=" + filepath.Join(root, "compiler-receipt"),
	}, root)
}
