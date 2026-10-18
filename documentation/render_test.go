package documentation

import (
	"io/fs"
	"testing"

	"github.com/tylergannon/skgo/documentation/render"
)

// The gate: the source tree meets its contract, every directive resolves,
// every exported symbol has one page, no page writes a pin literal, and the
// committed, embedded rendered tree equals a fresh render.
func TestDocumentationRendersCleanAndCommitted(t *testing.T) {
	root, err := render.FindRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	out, problems := render.Render(root)
	for _, p := range problems {
		t.Error(p)
	}
	if len(problems) > 0 {
		t.Fatalf("%d problems; fix the sources, then go generate ./documentation", len(problems))
	}
	for rel, want := range out {
		got, err := fs.ReadFile(Rendered(), rel)
		if err != nil {
			t.Errorf("%s: not in the embedded tree; run go generate ./documentation", rel)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("%s: embedded tree differs from a fresh render; run go generate ./documentation", rel)
		}
	}
	_ = fs.WalkDir(Rendered(), ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if _, ok := out[p]; !ok {
				t.Errorf("%s: stale rendered file; run go generate ./documentation", p)
			}
		}
		return nil
	})
	if _, err := Versions(); err != nil {
		t.Error(err)
	}
}
