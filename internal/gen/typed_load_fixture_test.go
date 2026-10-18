package gen

import (
	"os"
	"path/filepath"
)

// The generated-handler witness shares only the real SDK's module pins.
func stageTypedLoadFixture(root, app string) error {
	if err := copySandboxTree(filepath.Join(root, "internal", "gen", "testdata", "typed-load-evolution"), app, nil); err != nil {
		return err
	}
	for _, file := range []string{"go.mod", "go.sum"} {
		if err := copySandboxFile(filepath.Join(root, "example", file), filepath.Join(app, file)); err != nil {
			return err
		}
	}
	if err := rewriteSkgoReplace(filepath.Join(app, "go.mod"), root); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(app, "web/package.json"), []byte(`{"type":"module"}`), 0644); err != nil {
		return err
	}
	return linkGeneratorKit(root, app)
}
