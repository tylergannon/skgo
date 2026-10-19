//go:build skgo_qualification

package adapter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dop251/goja"
)

// The adapter owns the final fold, after Vite has emitted JavaScript. Supply
// those chunks directly rather than rebuilding Kit to test chunk resolution.
func TestProductionFoldResolvesSharedTransitiveRemoteOutsideAppTSConfig(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	deps, err := filepath.Abs("../../example/web/node_modules")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(deps, filepath.Join(root, "node_modules")); err != nil {
		t.Fatal(err)
	}
	const config = `{"compilerOptions":{"moduleResolution":"bundler"},"exclude":["build","node_modules"]}`
	writeAdapterFixture(t, filepath.Join(root, "tsconfig.json"), config)
	const program = `import assert from 'node:assert/strict';
import {mkdirSync,writeFileSync,readFileSync,existsSync} from 'node:fs';
import {join} from 'node:path';
import {pathToFileURL} from 'node:url';
const {gojaEnvironment}=await import(pathToFileURL(process.argv[1]));
const root=process.argv[2];process.chdir(root);
const fold=gojaEnvironment(),out=join(root,'build/.goja');
const files={
 'bundle.js':"import {readDocument} from './store.svelte.js';globalThis.__skgo_transitive=()=>readDocument();",
 'store.svelte.js':"export {readDocument} from './api.js';",
 'api.js':"export {readDocument} from './shared-remote.js';",
 'shared-remote.js':"export function readDocument(){return globalThis.__skgo_remote('literal116/editorDocument');}",
 'unrelated-remote.js':"throw new Error('unrelated remote entry must not execute');"
};
const environment={config:{build:{outDir:'build/.goja'}}};let builds=0;
const builder={environments:{goja:environment},async build(received){
 assert.equal(received,environment);builds++;mkdirSync(out,{recursive:true});
 for(const [name,source]of Object.entries(files))writeFileSync(join(out,name),source);
}};
const before=readFileSync('tsconfig.json','utf8');
await fold.build({nodes:'',hooks:''},join(root,'build/ssr/bundle.js'),builder);
assert.equal(builds,1);assert.equal(readFileSync('tsconfig.json','utf8'),before);
assert.equal(existsSync(out),false,'fold must remove only its own intermediate output');
`
	runAdapterScript(t, "skgo-adapter/env.js", program, nil, root)
	source, err := os.ReadFile(filepath.Join(root, "build/ssr/bundle.js"))
	if err != nil {
		t.Fatal(err)
	}
	vm := goja.New()
	calls := 0
	if err := vm.Set("__skgo_remote", func(id string) string {
		calls++
		if id != "literal116/editorDocument" {
			t.Errorf("remote ID %q", id)
		}
		return "Literal transitive Go document 116"
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.RunString(string(source)); err != nil {
		t.Fatalf("folded artifact: %v", err)
	}
	read, ok := goja.AssertFunction(vm.Get("__skgo_transitive"))
	if !ok {
		t.Fatal("fold omitted entry callback")
	}
	value, err := read(goja.Undefined())
	if err != nil || value.String() != "Literal transitive Go document 116" || calls != 1 {
		t.Fatalf("folded callback: value=%v calls=%d err=%v", value, calls, err)
	}
}
