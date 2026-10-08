package adapter

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Kit's client guard belongs to Kit's Vite plugin. This exercises the actual
// adapter build rather than copying the guard's filename rules into Go.
func TestKitRejectsClientImportOfServerOnlyModuleInSkgoBuild(t *testing.T) {
	t.Parallel()
	deps, err := filepath.Abs("../../example/web/node_modules")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(deps, ".bin", "vp")); err != nil {
		t.Fatalf("example web dependencies are missing; run just install: %v", err)
	}
	root := t.TempDir()
	modules := filepath.Join(root, "node_modules")
	if err := os.Mkdir(modules, 0755); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(deps)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		// Kit writes generated $app types here. Sharing that directory with
		// the real example would change another test's type universe.
		if entry.Name() == "$app" || entry.Name() == ".vite-temp" {
			continue
		}
		if err := os.Symlink(filepath.Join(deps, entry.Name()), filepath.Join(modules, entry.Name())); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("package.json", `{"name":"skgo-server-only-proof","private":true,"type":"module"}`)
	write("vite.config.ts", `import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite-plus';
import skgo from '@skgo/sveltekit-adapter';
export default defineConfig({ plugins: [sveltekit({ adapter: skgo(), paths: { origin: 'http://127.0.0.1:8080' } })] });
`)
	write("skgo.remotes.json", `{"remotes":[],"loads":[],"actions":[],"endpoints":{},"build":{}}`)
	write("src/app.html", `<!doctype html><html lang="en"><head>%sveltekit.head%</head><body>%sveltekit.body%</body></html>`)
	write("src/lib/server/secret.ts", `export const receipt = 'server-only receipt';`)
	write("src/hooks.server.ts", `import { receipt } from './lib/server/secret';
import type { Handle } from '@sveltejs/kit';
export const handle: Handle = ({ event, resolve }) => { void receipt; return resolve(event); };
`)
	build := func() (string, error) {
		cmd := exec.Command(filepath.Join(deps, ".bin", "vp"), "build")
		cmd.Dir = root
		cmd.Env = fixtureBuildEnv()
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	write("src/routes/+page.svelte", `<script lang="ts">import { receipt } from '../lib/server/secret';</script><h1>{receipt}</h1>`)
	out, err := build()
	if err == nil {
		t.Fatalf("a client import of a server-only module built successfully:\n%s", out)
	}
	for _, want := range []string{
		"Cannot import", "src/lib/server/secret", "src/routes/+page.svelte", "could leak sensitive information",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Kit's build failure does not name %q:\n%s", want, out)
		}
	}
	// The allowed server-hook import is independent of this refused client
	// import. Check the shared successful build after running the private one,
	// so its generation and native build do not delay this client's guard.
	requireMinimalInputsBuild(t)
}
