package skgodocs

import (
	"testing"

	"github.com/tylergannon/skgo/documentation/render"
)

// Validate authored sources without requiring changes to the published snapshot.
func TestDocumentationSourcesRenderCleanly(t *testing.T) {
	root, err := render.FindRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	_, problems := render.Render(root)
	for _, p := range problems {
		t.Error(p)
	}
	if len(problems) > 0 {
		t.Fatalf("%d documentation source problems", len(problems))
	}
	if _, err := Versions(); err != nil {
		t.Error(err)
	}
}
