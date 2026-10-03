package adapter

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// Kit 3 generates a server instance around the manifest. Reading its route
// table must leave all server and application modules unevaluated.
func TestManifestReadDoesNotExecuteTheKitServer(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	module, err := filepath.Abs("skgo-adapter.js")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../../example/web")
	if err != nil {
		t.Fatal(err)
	}
	const source = `
import { writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
const { readKitManifest } = await import(pathToFileURL(process.argv[1]));
const dir = process.argv[2];
writeFileSync(join(dir, 'package.json'), '{"type":"module"}');
writeFileSync(join(dir, 'server.js'), 'throw new Error("manifest read executed server code");');
const header = "import { create_server } from './server.js';\n";
const manifest = 'const manifest = { app_dir: "_app", app_path: "_app", assets: new Set(["fixture.svg"]), mime_types: {}, client: {}, nodes: [() => import("./server.js")], remotes: {}, routes: [{ id: "/fixture" }] };';
const builder = {
 getBuildDirectory: () => dir,
 generateServerInstance: (file) => writeFileSync(file, header + manifest + '\nexport const server = create_server(manifest);\n')
};
const result = await readKitManifest(builder);
if (result.manifest.appDir !== '_app' || !result.manifest.assets.has('fixture.svg') ||
 result.manifest._.routes[0].id !== '/fixture' || result.manifest._.nodes.length !== 1) {
 throw new Error('the literal route/asset fixture was not extracted');
}
if (result.source.includes('create_server')) throw new Error('server wrapper survived');
for (const wrapper of [
 header + manifest + '\nexport const server = create_server(manifest, {});\n',
 "import { Server } from './server.js';\n" + manifest + '\nexport const server = new Server(manifest);\n',
 header + 'const renamed = {};\nexport const server = create_server(manifest);\n'
]) {
 builder.generateServerInstance = (file) => writeFileSync(file, wrapper);
 let rejected = false;
 try { await readKitManifest(builder); }
 catch (error) { rejected = error.message.includes('unknown shape'); }
 if (!rejected) throw new Error('an unknown wrapper was evaluated or accepted');
}
`
	cmd := exec.Command(node, "--input-type=module", "-e", source, module, t.TempDir())
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("manifest extraction: %v\n%s", err, out)
	}
}
