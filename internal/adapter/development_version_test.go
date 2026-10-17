package adapter

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevelopmentEnforcesPublishedKitPin(t *testing.T) {
	web, err := filepath.Abs("../../example/web")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ pin, installed string }{
		{"3.1.0", "3.0.0"},
		{"3.1.0", "2.55.0"},
		{"3.1.0", "3.0.0-next.28"},
		// Changing only the published peer declaration must change the gate.
		{"3.1.1", "3.1.0"},
	} {
		t.Run(tc.pin+"-rejects-"+tc.installed, func(t *testing.T) {
			app := t.TempDir()
			writeQueueFixture(t, filepath.Join(app, "node_modules/@sveltejs/kit/package.json"), []byte(`{"version":"`+tc.installed+`"}`))
			adapterRoot := filepath.Join(app, "adapter")
			for name, body := range pack(t) {
				if name == "package.json" && tc.pin != "3.1.0" {
					var pkg map[string]any
					if err := json.Unmarshal(body, &pkg); err != nil {
						t.Fatal(err)
					}
					pkg["peerDependencies"].(map[string]any)["@sveltejs/kit"] = tc.pin
					body, err = json.Marshal(pkg)
					if err != nil {
						t.Fatal(err)
					}
				}
				writeQueueFixture(t, filepath.Join(adapterRoot, name), body)
			}
			// Import from the installed frontend environment, then point the
			// real development adapter at the consumer's incompatible Kit.
			program := `
import { pathToFileURL } from 'node:url';
const { development } = await import(pathToFileURL(process.env.SKGO_DEV_MODULE).href);
process.chdir(process.env.SKGO_VERSION_APP);
try {
  development({config: {paths: {}}});
  throw new Error('development unexpectedly accepted incompatible Kit');
} catch (error) {
  const expected = 'skgo development requires @sveltejs/kit@' + process.env.SKGO_EXPECTED_PIN + ', installed ' + process.env.SKGO_VERSION + '.';
  if (error.message !== expected) throw error;
  console.log(error.message);
}
`
			cmd := exec.Command(filepath.Join(web, "node_modules/.bin/vp"), "exec", "node", "--input-type=module", "-e", program)
			cmd.Dir = web
			cmd.Env = append(os.Environ(), "SKGO_DEV_MODULE="+filepath.Join(adapterRoot, "skgo-adapter/dev.js"), "SKGO_VERSION_APP="+app, "SKGO_VERSION="+tc.installed, "SKGO_EXPECTED_PIN="+tc.pin, "SKGO_DEV_DIR="+filepath.Join(app, "dev"), "SKGO_DEV_ORIGIN=http://localhost:8080")
			out, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(out), "installed "+tc.installed) {
				t.Fatalf("development version guard: %v\n%s", err, out)
			}
			t.Log(strings.TrimSpace(string(out)))
		})
	}
}
