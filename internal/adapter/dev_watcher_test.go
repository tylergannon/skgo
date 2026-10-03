package adapter

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDevTemplateWatcherReportsAndRecoversFromPartialSave(t *testing.T) {
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
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import assert from 'node:assert/strict';
const { gojaDevUnchangedFiles } = await import(pathToFileURL(process.argv[1]));
const root = process.argv[2];
const file = join(root, 'app.html');
const watcher = new EventEmitter();
const errors = [], messages = [], rendered = [];
const server = { watcher, config: { root, logger: { error: e => errors.push(e) } }, ws: { send: m => messages.push(m) } };
gojaDevUnchangedFiles().configureServer(server);
watcher.on('all', (_, changed) => {
 if (changed !== file) return;
 const text = readFileSync(file, 'utf8');
 for (const tag of ['%sveltekit.head%', '%sveltekit.body%']) {
  if (!text.includes(tag)) throw new Error('app.html is missing ' + tag);
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
assert.equal(messages[0].err.message, 'app.html is missing %sveltekit.head%');
assert.equal(errors.length, 1);
assert.deepEqual(rendered, []);
save('%sveltekit.head%');
assert.equal(messages.length, 2);
assert.equal(messages[1].err.message, 'app.html is missing %sveltekit.body%');
save('%sveltekit.head% restored %sveltekit.body%');
assert.deepEqual(rendered, ['%sveltekit.head% restored %sveltekit.body%']);
// An unrelated plugin exception must not be swallowed by the template guard.
watcher.on('all', () => { throw new Error('unrelated watcher failure'); });
assert.throws(() => save('%sveltekit.head% next %sveltekit.body%'), /unrelated watcher failure/);
`
	cmd := exec.Command(node, "--input-type=module", "-e", source, module, root)
	cmd.Dir = app
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("dev template watcher: %v\n%s", err, out)
	}
}
