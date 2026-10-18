package adapter

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdapterRouteConfigIsRefusedAtBuild(t *testing.T) {
	fixture := prepareMinimalInputsApp(t)
	writeFixtureFile(t, filepath.Join(fixture, "web/src/routes/+page.ts"), `export const config = { region: 'antarctica' };`)
	generate := exec.Command("go", "generate", "./...")
	generate.Dir = fixture
	generate.Env = append(fixtureBuildEnv(), "GOWORK=off")
	if out, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate: %v\n%s", err, out)
	}
	build := exec.Command(filepath.Join(fixture, "web/node_modules/.bin/vp"), "build")
	build.Dir = filepath.Join(fixture, "web")
	build.Env = fixtureBuildEnv()
	output, err := build.CombinedOutput()
	if err == nil {
		t.Fatalf("unsupported config built successfully:\n%s", output)
	}
	for _, want := range []string{"route / exports unsupported adapter config (region)", "Remove config"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("build refusal missing %q:\n%s", want, output)
		}
	}
	t.Logf("expected adapter config refusal:\n%s", output)
}
