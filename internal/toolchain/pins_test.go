package toolchain

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Each pin has one file that enforces it. These tests keep the constants
// equal to those files, so the documentation and `skgo advice` can quote the
// constants and be right.

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestGoMatchesModule(t *testing.T) {
	for _, path := range []string{"../../go.mod", "../../go.work", "../../example/go.mod"} {
		want := "\ngo " + Go + "\n"
		if !strings.Contains("\n"+read(t, path), want) {
			t.Errorf("%s does not declare go %s", path, Go)
		}
	}
}

func TestKitAndSvelteMatchExampleApplication(t *testing.T) {
	var manifest struct {
		DevDependencies map[string]string `json:"devDependencies"`
		Dependencies    map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(read(t, "../../example/web/package.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	version := func(name string) string {
		if v, ok := manifest.DevDependencies[name]; ok {
			return v
		}
		return manifest.Dependencies[name]
	}
	if got := version("@sveltejs/kit"); got != Kit {
		t.Errorf("example/web pins @sveltejs/kit %q, toolchain.Kit is %q", got, Kit)
	}
	if got := version("svelte"); got != Svelte {
		t.Errorf("example/web pins svelte %q, toolchain.Svelte is %q", got, Svelte)
	}
	if got := version("vite"); got != "npm:@voidzero-dev/vite-plus-core@"+VitePlus {
		t.Errorf("example/web pins vite %q, toolchain.VitePlus is %q", got, VitePlus)
	}
}

// VitePlus is the frontend environment's binding Node constraint; the lockfile
// records the engines range of the exact release the example installs.
func TestNodeMatchesVitePlusEngines(t *testing.T) {
	entry := regexp.MustCompile(`(?m)^  vite-plus@` + regexp.QuoteMeta(VitePlus) + `:\n(?:    .*\n)*?    engines: \{node: (.+)\}$`)
	m := entry.FindStringSubmatch(read(t, "../../example/web/pnpm-lock.yaml"))
	if m == nil {
		t.Fatalf("example/web/pnpm-lock.yaml records no engines for vite-plus@%s", VitePlus)
	}
	if m[1] != Node {
		t.Errorf("vite-plus@%s declares node %q, toolchain.Node is %q", VitePlus, m[1], Node)
	}
}

func TestNewApplicationREADMEQuotesThePins(t *testing.T) {
	readme := read(t, "../../internal/newapp/gofiles/README.md.tmpl")
	for _, want := range []string{"Go " + Go, "Node `" + Node + "`", "VitePlus " + VitePlus} {
		if !strings.Contains(readme, want) {
			t.Errorf("README.md.tmpl does not say %q", want)
		}
	}
}

func TestPinsLookLikeVersions(t *testing.T) {
	semver := regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)
	for name, value := range map[string]string{"Kit": Kit, "Svelte": Svelte, "Go": Go, "VitePlus": VitePlus, "PNPM": PNPM} {
		if !semver.MatchString(value) {
			t.Errorf("%s = %q is not a version", name, value)
		}
	}
}
