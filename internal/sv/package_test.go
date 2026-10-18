package svaddon

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func TestPublishedPackageIsASelfContainedNativeAddon(t *testing.T) {
	tarball := map[string][]byte{}
	for _, name := range []string{"package.json", "sv-addon.js"} {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		tarball[name] = body
	}
	var pkg struct {
		Name             string            `json:"name"`
		Type             string            `json:"type"`
		Exports          map[string]any    `json:"exports"`
		Files            []string          `json:"files"`
		Dependencies     map[string]string `json:"dependencies"`
		PeerDependencies map[string]string `json:"peerDependencies"`
		DevDependencies  map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(tarball["package.json"], &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Name != "@skgo/sv" || pkg.Type != "module" {
		t.Errorf("package identity = %+v; want the @skgo/sv ESM package", pkg)
	}
	if pkg.Exports["."] != "./sv-addon.js" {
		t.Errorf("root export = %v; want ./sv-addon.js for sv's community add-on loader", pkg.Exports["."])
	}
	if !slices.Contains(pkg.Files, "sv-addon.js") {
		t.Errorf("published files = %v; want the exported sv-addon.js bundle", pkg.Files)
	}
	if pkg.PeerDependencies["sv"] == "" {
		t.Error("package does not declare the sv host as a peer")
	}
	if len(pkg.Dependencies) != 0 {
		t.Errorf("published add-on has runtime dependencies %v; build-only utilities must be bundled", pkg.Dependencies)
	}
	if pkg.DevDependencies["@sveltejs/sv-utils"] == "" {
		t.Error("source does not declare its build-only sv-utils dependency")
	}
	if bytes.Contains(tarball["sv-addon.js"], []byte("@sveltejs/sv-utils")) {
		t.Error("published add-on imports sv-utils instead of bundling it")
	}
}

func TestAddonKeepsTheGoEmbedPlaceholderTrackable(t *testing.T) {
	for _, name := range []string{"sv-addon.source.js", "sv-addon.js"} {
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte("!/build/.gitkeep")) {
			t.Errorf("%s does not preserve the generated Go embed placeholder", name)
		}
	}
}
