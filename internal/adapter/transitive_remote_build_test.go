package adapter

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionFoldResolvesSharedTransitiveRemoteOutsideAppTSConfig(t *testing.T) {
	t.Parallel()
	fixture := prepareMinimalInputsApp(t)
	web := filepath.Join(fixture, "web")
	tsconfigPath := filepath.Join(web, "tsconfig.json")
	tsconfig, err := os.ReadFile(tsconfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tsconfig), `"exclude": ["build", "node_modules"]`) || strings.Contains(string(tsconfig), "build/.goja") {
		t.Fatalf("fixture must retain the ordinary excluded-build tsconfig: %s", tsconfig)
	}
	writeFixtureFile(t, filepath.Join(web, "src", "routes", "editor.remote.go"), `package routes
import("context";"github.com/tylergannon/skgo")
func editorDocument(context.Context)(string,error){return "Literal transitive Go document 116",nil}
var _ = skgo.Query(editorDocument)
`)
	writeFixtureFile(t, filepath.Join(web, "src", "lib", "api.ts"), `import { editorDocument } from '../routes/editor.remote';
export function readDocument() { return editorDocument(); }
`)
	writeFixtureFile(t, filepath.Join(web, "src", "lib", "store.svelte.ts"), `import { readDocument } from './api';
class EditorStore {
 label = $state('Literal local store 116');
 async read() { return await readDocument(); }
}
export const editor = new EditorStore();
`)
	writeFixtureFile(t, filepath.Join(web, "src", "routes", "+page.svelte"), `<script lang="ts">
 import { editor } from '#lib/store.svelte';
</script>
<h1>{await editor.read()}</h1><p>{editor.label}</p>
`)
	// The current adapter defaults to inlining. Explicitly exercise the public
	// upstream splitting option so the historical shared-import precondition is
	// real, rather than assuming today's optimizer happens to emit that graph.
	configPath := filepath.Join(web, "vite.config.ts")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	const observer = `const sharedRemoteFixture = {
 name: 'shared-transitive-remote-fixture',
 configResolved(config) {
  config.environments.goja.build.rolldownOptions.output.codeSplitting = {
   groups: [{name:'editor-remote',test:id=>id.replaceAll('\\','/').includes('/src/routes/editor.remote.ts')}]
  };
 },
 generateBundle(_options, bundle) {
  if(this.environment.name!=='goja')return;
  const entry=bundle['bundle.js'];
  const targets=new Set(Object.values(bundle).filter(chunk=>chunk.type==='chunk' && !chunk.isEntry && Object.keys(chunk.modules).some(id=>id.replaceAll('\\','/').includes('/src/routes/editor.remote.ts'))).map(chunk=>chunk.fileName));
  if(entry?.type!=='chunk'||targets.size===0)throw new Error('fixture did not emit the real remote shared chunk: '+JSON.stringify(Object.values(bundle).filter(c=>c.type==='chunk').map(c=>({file:c.fileName,entry:c.isEntry,imports:c.imports,modules:Object.keys(c.modules)}))));
  const visited=new Set();
  function visit(name){if(visited.has(name))return;visited.add(name);for(const imported of bundle[name]?.imports??[])visit(imported);}
  for(const imported of entry.imports)visit(imported);
  const shared=[...targets].filter(name=>visited.has(name));
  if(shared.length===0)throw new Error('entry did not import the emitted remote shared chunk');
  console.log('GOJA_SHARED_REMOTE_IMPORT:'+shared.join(','));
 }
};
`
	updated := strings.Replace(string(config), "export default defineConfig({", observer+"\nexport default defineConfig({", 1)
	updated = strings.Replace(updated, "plugins: [", "plugins: [sharedRemoteFixture,", 1)
	writeFixtureFile(t, configPath, updated)
	generate := exec.Command("go", "generate", "./...")
	generate.Dir = fixture
	generate.Env = append(fixtureBuildEnv(), "GOWORK=off")
	if output, err := timedFixtureOutput(t, "transitive remote generation", generate); err != nil {
		t.Fatalf("generate fixture: %v\n%s", err, output)
	}
	stub, err := os.ReadFile(filepath.Join(web, "src", "routes", "editor.remote.ts"))
	if err != nil || !bytes.Contains(stub, []byte(`throw new Error("skgo: implemented in Go")`)) {
		t.Fatalf("remote body is not the generated throwing stub: %v\n%s", err, stub)
	}
	build := exec.Command(filepath.Join(web, "node_modules", ".bin", "vp"), "build")
	build.Dir = web
	build.Env = append(fixtureBuildEnv(), "GOWORK=off")
	output, err := timedFixtureOutput(t, "shared transitive production build", build)
	t.Logf("production build:\n%s", output)
	if err != nil {
		t.Fatalf("production build with excluded output failed: %v\n%s", err, output)
	}
	if !bytes.Contains(output, []byte("GOJA_SHARED_REMOTE_IMPORT:")) {
		t.Fatal("build never reached the shared remote import fixture assertion")
	}
	after, err := os.ReadFile(tsconfigPath)
	if err != nil || !bytes.Equal(after, tsconfig) {
		t.Fatalf("production build changed application tsconfig: %v\n%s", err, after)
	}
	if _, err := os.Stat(filepath.Join(web, "build", "ssr", "bundle.js")); err != nil {
		t.Fatalf("no final folded artifact: %v", err)
	}
	writeFixtureFile(t, filepath.Join(fixture, "internal", "skgo", "transitive_production_test.go"), `package skgo_test
import (
 "net/http/httptest"
 "os"
 "strings"
 "testing"
 "github.com/tylergannon/skgo"
 generated "github.com/tylergannon/skgo/example/internal/skgo"
)
func TestTransitiveRemoteProductionHandler(t *testing.T) {
 dist:=os.DirFS("../../web/build")
 manifest,err:=skgo.ReadManifest(dist);if err!=nil{t.Fatal(err)}
 loads,err:=skgo.NewLoads(manifest.LoadConfig("http://127.0.0.1:8080"),generated.Loads()...);if err!=nil{t.Fatal(err)}
 remotes,err:=skgo.NewRemotes(manifest.RemoteConfig("http://127.0.0.1:8080"),generated.Remotes()...);if err!=nil{t.Fatal(err)}
 renderer,err:=skgo.NewSSR(dist,manifest,loads,remotes,skgo.SSROptions{Runtimes:1});if err!=nil{t.Fatal(err)}
 handler,err:=skgo.NewStaticHandler(dist,skgo.WithSSR(renderer));if err!=nil{t.Fatal(err)}
 response:=httptest.NewRecorder()
 handler.ServeHTTP(response,httptest.NewRequest("GET","http://127.0.0.1:8080/",nil))
 if response.Code!=200||!strings.Contains(response.Body.String(),"Literal transitive Go document 116")||!strings.Contains(response.Body.String(),"Literal local store 116") {
  t.Fatalf("production transitive handler: status=%d body=%s",response.Code,response.Body.String())
 }
}
`)
	consumer := exec.Command("go", "test", "-count=1", "-v", "./internal/skgo", "-run", "^TestTransitiveRemoteProductionHandler$")
	consumer.Dir = fixture
	consumer.Env = append(fixtureBuildEnv(), "GOWORK=off")
	output, err = timedFixtureOutput(t, "production transitive handler", consumer)
	t.Logf("Go/goja handler:\n%s", output)
	if err != nil || bytes.Contains(output, []byte("--- SKIP:")) || !bytes.Contains(output, []byte("--- PASS: TestTransitiveRemoteProductionHandler")) {
		t.Fatalf("production handler did not render literal Go remote result: %v\n%s", err, output)
	}
}
