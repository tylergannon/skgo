package adapter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemotePathUsesEvaluatedKitConfiguration(t *testing.T) {
	web := t.TempDir()
	installed, err := filepath.Abs("../../example/web/node_modules")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(installed, filepath.Join(web, "node_modules")); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(web, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("package.json", `{"type":"module"}`)
	if err := os.Mkdir(filepath.Join(web, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	write("src/app.html", `<html><head>%sveltekit.head%</head><body>%sveltekit.body%</body></html>`)
	write("endpoint.ts", `export const appDir = process.env.SKGO_TEST_APP_DIR || 'configured/assets';`)
	write("vite.config.ts", `import { sveltekit } from '@sveltejs/kit/vite';
import { appDir } from './endpoint';
export default { plugins: [sveltekit({ appDir, paths: {base: '/shop'} })] };`)
	t.Setenv("SKGO_TEST_APP_DIR", "")
	if got, err := RemotePath(t.Context(), web); err != nil || got != "configured/assets/remote/" {
		t.Fatalf("path=%q err=%v", got, err)
	}
	t.Setenv("SKGO_TEST_APP_DIR", "changed/assets")
	if got, err := RemotePath(t.Context(), web); err != nil || got != "changed/assets/remote/" {
		t.Fatalf("path=%q err=%v", got, err)
	}
	write("endpoint.ts", `export const appDir = '_app';`)
	if got, err := RemotePath(t.Context(), web); err != nil || got != "_app/remote/" {
		t.Fatalf("path=%q err=%v", got, err)
	}
}
