package gen

import (
	"os"
	"path/filepath"
	"sync"
)

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

// Shared parameter tests own their routes and domain; copying every remote,
// hook and load of the example makes each transition analyse unrelated code.
func stageSharedModule(root, app string) error {
	for _, file := range []string{"go.mod", "go.sum", "web/src/params.go", "web/src/params.ts"} {
		if err := copySandboxFile(filepath.Join(root, "example", filepath.FromSlash(file)), filepath.Join(app, filepath.FromSlash(file))); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(app, "internal/app"), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(app, "internal/app/locals.go"), []byte("package app\ntype Locals struct{Visitor string}\n"), 0644); err != nil {
		return err
	}
	if err := rewriteSkgoReplace(filepath.Join(app, "go.mod"), root); err != nil {
		return err
	}
	return linkGeneratorKit(root, app)
}

func stageEvolvedModule(root, app string) error {
	if err := stageSharedModule(root, app); err != nil {
		return err
	}
	for _, route := range []string{"optional", "stream", "contact"} {
		if err := copySandboxTree(filepath.Join(root, "example/web/src/routes", route), filepath.Join(app, "web/src/routes", route), nil); err != nil {
			return err
		}
	}
	for _, file := range []string{"internal/skgo/client/skgo_gen.go"} {
		if err := copySandboxFile(filepath.Join(root, "example", file), filepath.Join(app, file)); err != nil {
			return err
		}
	}
	for _, route := range []string{"todos/[id]", "typed-load/[number=Order]"} {
		path := filepath.Join(app, "web/src/routes", route, "+page.svelte")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte("<p>caller</p>"), 0644); err != nil {
			return err
		}
	}
	path := filepath.Join(app, "internal/skgo/config.go")
	return os.WriteFile(path, []byte("package skgo\n//go:generate go tool skgo generate --web ../../web --locals-package github.com/tylergannon/skgo/example/internal/app\n"), 0644)
}

func prepareCheckModule(name string) (string, error) {
	root, err := repoRoot()
	if err != nil {
		return "", err
	}
	app := filepath.Join(packageTemp, name)
	if err := stageSharedModule(root, app); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(app, "web/package.json"), []byte("{\"type\":\"module\"}\n"), 0644); err != nil {
		return "", err
	}
	route := filepath.Join(app, "web/src/routes/todos")
	if err := os.MkdirAll(route, 0755); err != nil {
		return "", err
	}
	source := `package todos
import("context";"github.com/tylergannon/skgo")
type Rename struct {
 ID string ` + "`json:\"id\"`" + `
}
func rename(context.Context,Rename)(string,error){return "fixture",nil}
var _=skgo.Query(rename)
`
	if err := os.WriteFile(filepath.Join(route, "todos.remote.go"), []byte(source), 0644); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(app, "internal/skgo"), 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(app, "internal/skgo/config.go"), []byte("package skgo\n//go:generate go tool skgo generate --web ../../web --locals-package github.com/tylergannon/skgo/example/internal/app\n"), 0644); err != nil {
		return "", err
	}
	cfg := fixtureConfig(Config{Web: filepath.Join(app, "web"), Out: filepath.Join(app, "internal/skgo"), Logf: func(string, ...any) {}})
	if err := Run(cfg); err != nil {
		return "", err
	}
	return app, nil
}

// CLI and in-process checks need independently mutable source, but not three
// identical initial generations. Copy the once-generated pristine tree.
var preparedCheckModule = sync.OnceValues(func() (string, error) { return prepareCheckModule("check-template") })

func cloneCheckModule(name string) (string, error) {
	template, err := preparedCheckModule()
	if err != nil {
		return "", err
	}
	app := filepath.Join(packageTemp, name)
	if err := copySandboxTree(template, app, map[string]bool{"web/node_modules": true}); err != nil {
		return "", err
	}
	root, err := repoRoot()
	if err != nil {
		return "", err
	}
	if err := linkGeneratorKit(root, app); err != nil {
		return "", err
	}
	return app, nil
}
