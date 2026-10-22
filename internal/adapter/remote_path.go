package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
)

// RemotePath reads Kit's validated options from Vite's evaluated configuration.
// This honors imported values and environment-dependent application settings.
func RemotePath(ctx context.Context, web string) (string, error) {
	web, err := filepath.Abs(web)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "node", "--input-type=module", "--eval", `
import { pathToFileURL } from 'node:url';
console.log = (...args) => console.error(...args);
const { extract_svelte_config } = await import(pathToFileURL(process.argv[1]));
const { import_peer } = await import(pathToFileURL(process.argv[2]));
const vite = await import_peer('vite', process.cwd());
// Loading the config evaluates Kit's plugin factory and validates its options.
// Resolving a complete build would run sync hooks and rewrite shared $app types.
const loaded = await vite.loadConfigFromFile({ command: 'build', mode: process.env.MODE ?? 'production' });
if (!loaded) throw new Error('Vite configuration is required');
async function flatten(value) {
 value = await value;
 return Array.isArray(value) ? (await Promise.all(value.map(flatten))).flat() : value ? [value] : [];
}
const plugins = await flatten(loaded.config.plugins);
if (!plugins.some(p => p.name === 'vite-plugin-sveltekit-setup')) throw new Error('SvelteKit plugin is required');
const config = extract_svelte_config({ plugins });
process.stdout.write(JSON.stringify(config.appDir + '/remote/'));
`, filepath.Join(web, "node_modules/@sveltejs/kit/src/core/config/index.js"), filepath.Join(web, "node_modules/@sveltejs/kit/src/utils/import.js"))
	cmd.Dir = web
	data, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("reading Kit remote path: %s", exit.Stderr)
		}
		return "", fmt.Errorf("reading Kit remote path: %w", err)
	}
	var path string
	if err := json.Unmarshal(data, &path); err != nil {
		return "", fmt.Errorf("reading Kit remote path: %w", err)
	}
	return path, nil
}
