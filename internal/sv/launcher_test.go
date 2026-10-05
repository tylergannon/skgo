package svaddon

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Load the shipped bundle unchanged and run its bundled sv-utils transforms
// against fixture files. Only sv's registration surface is doubled here.
func TestGeneratedLaunchersUseProjectLocalVitePlusAndPreserveAuthoredConfig(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node is required to load the published native add-on: %v", err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "sv"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"package.json":                 `{"type":"module"}`,
		"node_modules/sv/package.json": `{"type":"module","exports":"./index.js"}`,
		"node_modules/sv/index.js":     `export const defineAddon = (addon) => addon; export const defineAddonOptions = () => ({ add() { return this; }, build() { return {}; } });`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bundle, err := os.ReadFile("sv-addon.js")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sv-addon.js"), bundle, 0o644); err != nil {
		t.Fatal(err)
	}
	for file, body := range map[string]string{
		"package.json":          `{"name":"fixture","scripts":{},"devDependencies":{}}`,
		".gitignore":            "\n/build\n",
		"vite.config.ts":        `import { defineConfig } from 'vite'; export default defineConfig({ plugins: [] });`,
		"playwright.config.ts":  `import { defineConfig } from "@playwright/test"; export default defineConfig({ webServer: { command: "npm run build && npm run preview", port: 4173, timeout: 60000 }, testMatch: "**/*.e2e.{ts,js}", reporter: "json" });`,
		".mcp.json":             `{"mcpServers":{"svelte":{"type":"stdio","command":"npx","env":{"TOKEN":"keep"},"args":["-y","@sveltejs/mcp"]},"other":{"command":"custom"}}}`,
		".cursor/mcp.json":      `{"mcpServers":{"svelte":{"command":"custom-svelte","args":["serve"]}}}`,
		".gemini/settings.json": `{"mcpServers":{"svelte":{"url":"https://mcp.svelte.dev/mcp","type":"http"}}}`,
		".vscode/mcp.json":      `{"servers":{"svelte":{"type":"stdio","command":"npx","args":["-y","@sveltejs/mcp"]}}}`,
		".claude/skills/svelte-code-writer/SKILL.md": "Custom instructions mentioning npx @sveltejs/mcp are authored.\n",
	} {
		name := filepath.Join(root, "web", file)
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	harness := `
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
const root = process.cwd();
const addon = (await import(pathToFileURL(path.join(root, 'sv-addon.js')))).default;
const files = new Set(['package.json','.gitignore','vite.config.ts','playwright.config.ts','.mcp.json','.cursor/mcp.json','.gemini/settings.json','.vscode/mcp.json','.claude/skills/svelte-code-writer/SKILL.md']);
const sv = {
  devDependency() {},
  file(name, transform) {
    if (!files.has(name)) return;
    const filename = path.join(root, 'web', name);
    const original = fs.readFileSync(filename, 'utf8');
    const next = transform(original);
    if (next !== false && next !== undefined) fs.writeFileSync(filename, next);
  }
};
const addonFile = { package: 'package.json', gitignore: '.gitignore' };
const options = { starter: 'minimal', adapter: '0.17.0', name: 'sample', origin: encodeURIComponent('http://127.0.0.1:19080') };
function apply() { addon.run({ sv, file: addonFile, cwd: path.join(root,'web'), options, language: 'ts' }); }
apply();
let config = fs.readFileSync(path.join(root,'web/playwright.config.ts'),'utf8');
assert.match(config, /command:\s*["']cd \.\. && just serve["']/);
assert.match(config, /url:\s*Reflect\.get\(globalThis, ["']process["']\)\?\.env\?\.ORIGIN\s*\|\|\s*["']http:\/\/127\.0\.0\.1:19080["']/);
assert.match(config, /baseURL:\s*Reflect\.get\(globalThis, ["']process["']\)\?\.env\?\.ORIGIN\s*\|\|\s*["']http:\/\/127\.0\.0\.1:19080["']/);
assert.doesNotMatch(config, /port:\s*4173/);
assert.match(config, /timeout:\s*60000/);
assert.match(config, /testMatch:\s*["']\*\*\/\*\.e2e\.\{ts,js\}["']/);
assert.match(config, /reporter:\s*["']json["']/);
let claude = JSON.parse(fs.readFileSync(path.join(root,'web/.mcp.json'),'utf8'));
assert.deepEqual(claude.mcpServers.svelte.args, ['dlx','@sveltejs/mcp']);
assert.equal(claude.mcpServers.svelte.command, './node_modules/.bin/vp');
assert.deepEqual(claude.mcpServers.svelte.env, { TOKEN: 'keep' });
assert.deepEqual(claude.mcpServers.other, { command: 'custom' });
assert.deepEqual(JSON.parse(fs.readFileSync(path.join(root,'web/.vscode/mcp.json'),'utf8')).servers.svelte.args, ['dlx','@sveltejs/mcp']);
assert.deepEqual(JSON.parse(fs.readFileSync(path.join(root,'web/.cursor/mcp.json'),'utf8')).mcpServers.svelte.args, ['serve']);
assert.equal(JSON.parse(fs.readFileSync(path.join(root,'web/.cursor/mcp.json'),'utf8')).mcpServers.svelte.command, 'custom-svelte');
assert.equal(JSON.parse(fs.readFileSync(path.join(root,'web/.gemini/settings.json'),'utf8')).mcpServers.svelte.url, 'https://mcp.svelte.dev/mcp');
assert.equal(fs.readFileSync(path.join(root,'web/.claude/skills/svelte-code-writer/SKILL.md'),'utf8'), 'Custom instructions mentioning npx @sveltejs/mcp are authored.\n');
const before = new Map([...files].map(name => [name, fs.readFileSync(path.join(root,'web',name),'utf8')]));
apply();
for (const [name, body] of before) assert.equal(fs.readFileSync(path.join(root,'web',name),'utf8'), body, name + ' changed on a second add');
function rejectUnchanged(source, expected) {
  const configPath = path.join(root,'web/playwright.config.ts');
  fs.writeFileSync(configPath, source);
  assert.throws(apply, expected);
  assert.equal(fs.readFileSync(configPath,'utf8'), source, 'unsupported config changed before the diagnostic');
}
rejectUnchanged("import { defineConfig } from '@playwright/test'; export default customFactory({ webServer: { command: 'npm run build && npm run preview', port: 4173 } });", /single-argument imported defineConfig\(object\)/);
rejectUnchanged("import { defineConfig } from '@playwright/test'; export default defineConfig({ webServer: { command: 'npm run build && npm run preview', port: 4173 } }, { webServer: { command: 'custom' } });", /single-argument imported defineConfig\(object\)/);
rejectUnchanged("import { defineConfig } from '@playwright/test'; export default defineConfig({ webServer: { command: 'npm run build && npm run preview', port: 4173, ...customServer } });", /spread, computed, or duplicate launcher property/);
rejectUnchanged("import { defineConfig } from '@playwright/test'; export default defineConfig({ webServer: { command: 'npm run build && npm run preview', port: 4173, command: 'echo custom' } });", /spread, computed, or duplicate launcher property/);
rejectUnchanged("import { defineConfig } from '@playwright/test'; export default defineConfig({ ['webServer']: { command: 'custom' }, webServer: { command: 'npm run build && npm run preview', port: 4173 } });", /spread, computed, or duplicate launcher property/);
rejectUnchanged("import { defineConfig } from '@playwright/test'; export default defineConfig({ webServer: { ['command']: 'custom', command: 'npm run build && npm run preview', port: 4173 } });", /spread, computed, or duplicate launcher property/);
rejectUnchanged("import { defineConfig } from '@playwright/test'; export default defineConfig({ webServer: { command: 'npm run build && npm run preview', port: 4173 }, use: { ...customUse } });", /use.*spread\/computed\/duplicate baseURL/);
rejectUnchanged("import { defineConfig } from '@playwright/test'; export default defineConfig({ webServer: { command: 'npm run build && npm run preview', port: 4173 }, use: { ['baseURL']: 'custom' } });", /use.*spread\/computed\/duplicate baseURL/);
`
	cmd := exec.Command(node, "--input-type=module", "-e", harness)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native launcher contract: %v\n%s", err, out)
	}
}
