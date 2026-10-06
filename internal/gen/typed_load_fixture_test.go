package gen

import "path/filepath"

// This mutable application owns its matcher and consumers. Only dependency
// bootstrap comes from the example, whose module identity is part of the test.
func stageTypedLoadEvolutionFixture(root, app string) error {
	if err := copySandboxTree(filepath.Join(root, "internal", "gen", "testdata", "typed-load-evolution"), app, nil); err != nil {
		return err
	}
	for _, file := range []string{
		"go.mod", "go.sum", "web/package.json", "web/vite.config.ts",
		"web/tsconfig.json", "web/src/app.html",
	} {
		if err := copySandboxFile(filepath.Join(root, "example", filepath.FromSlash(file)), filepath.Join(app, filepath.FromSlash(file))); err != nil {
			return err
		}
	}
	if err := rewriteSkgoReplace(filepath.Join(app, "go.mod"), root); err != nil {
		return err
	}
	if err := replaceOnce(filepath.Join(app, "web", "package.json"), "link:../../internal/adapter", "link:"+filepath.ToSlash(filepath.Join(root, "internal", "adapter"))); err != nil {
		return err
	}
	return linkGeneratorKit(root, app)
}
