package gen

import "path/filepath"

// stagePrerenderFixture shares only build bootstrap from the example. Each
// checked-in fixture owns its application sources and its separate build.
func stagePrerenderFixture(root, app, name string) error {
	if err := copySandboxTree(filepath.Join(root, "internal", "gen", "testdata", name), app, nil); err != nil {
		return err
	}
	for _, file := range []struct{ source, destination string }{
		{"go.mod", "go.mod"},
		{"go.sum", "go.sum"},
		{"web/package.json", "ui/package.json"},
		{"web/vite.config.ts", "ui/vite.config.ts"},
		{"web/tsconfig.json", "ui/tsconfig.json"},
		{"web/src/app.html", "ui/src/app.html"},
	} {
		if err := copySandboxFile(filepath.Join(root, "example", filepath.FromSlash(file.source)), filepath.Join(app, filepath.FromSlash(file.destination))); err != nil {
			return err
		}
	}
	if err := rewriteSkgoReplace(filepath.Join(app, "go.mod"), root); err != nil {
		return err
	}
	if err := replaceOnce(filepath.Join(app, "go.mod"), "ignore ./web/node_modules", "ignore ./ui/node_modules"); err != nil {
		return err
	}
	return replaceOnce(filepath.Join(app, "ui", "package.json"), "link:../../internal/adapter", "link:"+filepath.ToSlash(filepath.Join(root, "internal", "adapter")))
}
