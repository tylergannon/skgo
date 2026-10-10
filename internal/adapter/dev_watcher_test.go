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
