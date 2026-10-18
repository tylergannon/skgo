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

func TestNewApplicationREADMEQuotesThePins(t *testing.T) {
	readme := read(t, "../../internal/newapp/gofiles/README.md.tmpl")
	for _, want := range []string{"Go " + Go, "Node " + NodeMinimum + " or newer", "VitePlus " + VitePlus} {
		if !strings.Contains(readme, want) {
			t.Errorf("README.md.tmpl does not say %q", want)
		}
	}
}

func TestPinsLookLikeVersions(t *testing.T) {
	semver := regexp.MustCompile(`^\d+\.\d+(\.\d+)?$`)
	for name, value := range map[string]string{"Kit": Kit, "Svelte": Svelte, "Go": Go, "NodeMinimum": NodeMinimum, "VitePlus": VitePlus, "PNPM": PNPM} {
		if !semver.MatchString(value) {
			t.Errorf("%s = %q is not a version", name, value)
		}
	}
	data, err := json.Marshal(Current())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"kit"`, `"svelte"`, `"go"`, `"node"`, `"vitePlus"`, `"pnpm"`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("versions JSON lacks %s: %s", key, data)
		}
	}
}

func TestNodeRangeMatchesQualifiedVitePlus(t *testing.T) {
	var manifest struct {
		Version string
		Engines struct{ Node string }
	}
	if err := json.Unmarshal([]byte(read(t, "../../example/web/node_modules/vite-plus/package.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != VitePlus || manifest.Engines.Node != Node {
		t.Fatalf("qualified VitePlus=%s Node=%q, pins=%s %q", manifest.Version, manifest.Engines.Node, VitePlus, Node)
	}
	if Node != "^22.18.0 || ^24.11.0 || >=26.0.0" {
		t.Fatalf("unexpected qualified Node range %q", Node)
	}
}

func TestKitMatchesQueuePatch(t *testing.T) {
	if !strings.Contains(read(t, "../kitpatch/kitpatch.go"), `ownedPatchName = "skgo-kit-`+Kit+`-queue.patch"`) {
		t.Fatal("Kit pin and owned compatibility patch disagree")
	}
	var metadata struct{ Version string }
	if err := json.Unmarshal([]byte(read(t, "../adapter/skgo-adapter/compat/kit-"+Kit+"-queue.json")), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Version != Kit {
		t.Fatal("Kit pin and adapter compatibility metadata disagree")
	}
}
