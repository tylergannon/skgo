package tailwind

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/templates"
	"github.com/tylergannon/skgo/templateapi"
)

func fixture(t *testing.T, files map[string]string) (templateapi.Project, plugin, *int) {
	t.Helper()
	root := t.TempDir()
	project := templateapi.Project{Root: root, Web: filepath.Join(root, "web"), Name: "TailwindFixture", Module: "example.test/tailwind"}
	base := map[string]string{
		"package.json": `{"name":"fixture","type":"module","devDependencies":{"vite-plus":"1.3.0"},"devEngines":{"packageManager":{"name":"pnpm","version":"12.14.0"}}}`,
		"vite.config.ts": `import { defineConfig } from 'vite-plus';
import { sveltekit } from '@sveltejs/kit/vite';
// Authored configuration must remain intact.
export default defineConfig({ plugins: [sveltekit()], server: { strictPort: true } });
`,
	}
	for name, source := range files {
		base[name] = source
	}
	for name, source := range base {
		write(t, filepath.Join(project.Web, name), source)
	}
	installs := new(int)
	p := plugin{version: "test", install: func(ctx context.Context, web string) error {
		if web != project.Web {
			t.Fatalf("installation cwd %s", web)
		}
		*installs++
		for _, name := range []string{"tailwindcss", "@tailwindcss/vite"} {
			write(t, filepath.Join(web, "node_modules", filepath.FromSlash(name), "package.json"), `{"version":"4.3.3"}`)
		}
		write(t, filepath.Join(web, "pnpm-lock.yaml"), "literal installed dependencies\n")
		return ctx.Err()
	}}
	return project, p, installs
}
func write(t *testing.T, path, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
}
func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestTailwindSetupPreservesFrontendAndScopesEveryGoLocation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, layout, stylesheet, relative, exclude string
		existing                                    bool
	}{
		{"fresh", "src/routes/+layout.svelte", "src/routes/layout.css", "./layout.css", "../../**/*.go", false},
		{"edited-existing", "src/routes/+layout.svelte", "src/styles/theme.css", "../styles/theme.css", "../../**/*.go", true},
		{"nondefault-layout-and-css", "frontend/views/+layout.svelte", "styles/site.css", "../../styles/site.css", "../**/*.go", true},
		{"frontend-root-css", "src/routes/+layout.svelte", "app.css", "../../app.css", "./**/*.go", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{}
			authoredCSS := "/* Handwritten theme */\n.existing { color: rgb(17, 34, 51); }\n"
			authoredLayout := "<script lang=\"ts\">\n  // Keep the user's script.\n  import '" + tc.relative + "';\n  let { children } = $props();\n</script>\n<section data-authored=\"kept\">{@render children()}</section>\n"
			if tc.existing {
				files[tc.layout] = authoredLayout
				files[tc.stylesheet] = authoredCSS
			}
			project, p, installs := fixture(t, files)
			priorVite := read(t, filepath.Join(project.Web, "vite.config.ts"))
			options := map[string]string{"layout": tc.layout}
			// Auto-discovery must resolve the actual import, including ../ segments.
			result, err := p.Apply(context.Background(), templateapi.Request{Addon: "tailwindcss", Project: project, Options: options})
			if err != nil {
				t.Fatal(err)
			}
			if *installs != 1 {
				t.Fatalf("installations %d", *installs)
			}
			css := read(t, filepath.Join(project.Web, tc.stylesheet))
			if !strings.Contains(css, `@import "tailwindcss";`) || !strings.Contains(css, `@source not "`+tc.exclude+`";`) {
				t.Fatalf("incorrect actual stylesheet scope:\n%s", css)
			}
			if tc.existing && !strings.Contains(css, authoredCSS) {
				t.Fatal("handwritten CSS was changed")
			}
			layout := read(t, filepath.Join(project.Web, tc.layout))
			if tc.existing && layout != authoredLayout {
				t.Fatalf("existing layout changed:\n%s", layout)
			}
			vite := read(t, filepath.Join(project.Web, "vite.config.ts"))
			if strings.Replace(strings.TrimPrefix(vite, "import skgoTailwindcss from \"@tailwindcss/vite\";\n"), "skgoTailwindcss(), ", "", 1) != priorVite {
				t.Fatalf("handwritten Vite config changed:\n%s", vite)
			}
			if strings.Contains(vite, "watch") || strings.Contains(vite, "ignored") {
				t.Fatal("add-on changed watcher policy")
			}
			var pkg struct{ DevDependencies map[string]string }
			if err := json.Unmarshal([]byte(read(t, filepath.Join(project.Web, "package.json"))), &pkg); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"tailwindcss", "@tailwindcss/vite"} {
				if pkg.DevDependencies[name] != "4.3.3" {
					t.Fatalf("dependency %s=%q", name, pkg.DevDependencies[name])
				}
			}
			if len(result.Changed) == 0 {
				t.Fatal("no changed paths reported")
			}
			before := map[string]string{}
			for _, name := range []string{tc.stylesheet, tc.layout, "vite.config.ts", "package.json", "pnpm-lock.yaml"} {
				before[name] = read(t, filepath.Join(project.Web, name))
			}
			second, err := p.Apply(context.Background(), templateapi.Request{Addon: "tailwindcss", Project: project, Options: options})
			if err != nil || len(second.Changed) != 0 || *installs != 1 {
				t.Fatalf("second application changed setup: %+v installs=%d err=%v", second, *installs, err)
			}
			for name, want := range before {
				if got := read(t, filepath.Join(project.Web, name)); got != want {
					t.Fatalf("duplicate setup changed %s", name)
				}
			}
		})
	}
}

func TestTailwindConflictsAndCancellationPrecedeWrites(t *testing.T) {
	t.Parallel()
	for name, files := range map[string]map[string]string{
		"legacy dependency":           {"package.json": `{"devDependencies":{"tailwindcss":"^3.4.0"}}`},
		"legacy CSS":                  {"src/routes/+layout.svelte": "<script>import '../app.css';</script>", "src/app.css": "@tailwind utilities;\n"},
		"dynamic config":              {"vite.config.ts": "export default () => ({plugins: []});\n"},
		"multiple stylesheet imports": {"src/routes/+layout.svelte": "<script>import '../a.css';import '../b.css';</script>"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			project, p, installs := fixture(t, files)
			before := map[string]string{}
			for name := range files {
				before[name] = read(t, filepath.Join(project.Web, name))
			}
			before["package.json"] = read(t, filepath.Join(project.Web, "package.json"))
			before["vite.config.ts"] = read(t, filepath.Join(project.Web, "vite.config.ts"))
			result, err := p.Apply(context.Background(), templateapi.Request{Addon: "tailwindcss", Project: project})
			if err == nil || len(result.Changed) != 0 || *installs != 0 {
				t.Fatalf("conflict mutated app: %+v installs=%d err=%v", result, *installs, err)
			}
			for name, want := range before {
				if got := read(t, filepath.Join(project.Web, name)); got != want {
					t.Fatalf("conflict changed %s", name)
				}
			}
		})
	}
	project, p, installs := fixture(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := p.Apply(ctx, templateapi.Request{Addon: "tailwindcss", Project: project})
	if !errors.Is(err, context.Canceled) || len(result.Changed) != 0 || *installs != 0 {
		t.Fatalf("cancelled setup: %+v %v", result, err)
	}
}

func TestTailwindReusesConfiguredPluginAndExplicitStylesheet(t *testing.T) {
	t.Parallel()
	const config = "import customTailwind from '@tailwindcss/vite';\nexport default {plugins:[customTailwind()], server:{host:true}};\n"
	project, p, _ := fixture(t, map[string]string{"vite.config.ts": config, "src/routes/+layout.svelte": "<script>import '../a.css';import '../b.css';let {children}=$props();</script>{@render children()}", "src/a.css": "/* @import 'tailwindcss'; @source not '../**/*.go'; */\n.authored {display:block}\n"})
	req := templateapi.Request{Addon: "tailwindcss", Project: project, Options: map[string]string{"stylesheet": "src/a.css"}}
	if _, err := p.Apply(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(project.Web, "vite.config.ts")); got != config {
		t.Fatalf("existing plugin changed: %s", got)
	}
	css := read(t, filepath.Join(project.Web, "src/a.css"))
	if strings.Count(css, "@import \"tailwindcss\"") != 1 || !strings.Contains(css, "@source not \"../**/*.go\";") {
		t.Fatalf("comments mistaken for active setup: %s", css)
	}
}

func TestTailwindTemplateCompositionUsesSameOptionalAddon(t *testing.T) {
	t.Parallel()
	project, p, _ := fixture(t, nil)
	registry, err := templates.DiscoverFiles(nil, "test", p, recipe{})
	if err != nil {
		t.Fatal(err)
	}
	direct, err := registry.Add("tailwindcss", nil)
	if err != nil {
		t.Fatal(err)
	}
	composed, err := registry.Template("styled", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(direct) != 1 || len(composed) != 1 || !reflect.DeepEqual(direct[0].Options, composed[0].Options) || direct[0].Name != composed[0].Name {
		t.Fatalf("composition differs: %+v %+v", direct, composed)
	}
	before := read(t, filepath.Join(project.Web, "package.json"))
	if strings.Contains(before, "tailwind") {
		t.Fatal("optional discovery mutated ordinary app")
	}
	var output bytes.Buffer
	if err := templates.Apply(context.Background(), composed, project, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "applied tailwindcss") {
		t.Fatal(output.String())
	}
}

type recipe struct{}

func (recipe) Describe() templateapi.Descriptor {
	return templateapi.Descriptor{ID: "fixture", Version: "test", SkgoVersion: "test", Templates: []templateapi.Template{{Name: "styled", Summary: "Styled fixture", Steps: []templateapi.Step{{Addon: "tailwindcss"}}}}}
}
func (recipe) Apply(context.Context, templateapi.Request) (templateapi.Result, error) {
	panic("template composition must invoke its add-on step")
}

func TestTailwindInstallationFailureReportsPartialChangesAndRetries(t *testing.T) {
	t.Parallel()
	project, p, installs := fixture(t, nil)
	install := p.install
	p.install = func(context.Context, string) error { return errors.New("literal install failure") }
	req := templateapi.Request{Addon: "tailwindcss", Project: project}
	result, err := p.Apply(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "literal install failure") || len(result.Changed) == 0 {
		t.Fatalf("lost partial changes: %+v %v", result, err)
	}
	p.install = install
	result, err = p.Apply(context.Background(), req)
	if err != nil || *installs != 1 {
		t.Fatalf("failed installation was not retried: %+v installs=%d err=%v", result, *installs, err)
	}
}

func TestTailwindViteConfigurationPreservesStaticShapesAndRejectsOverrides(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		"export default {server:{strictPort:true}};",
		`import {defineConfig,lazyPlugins} from "vite-plus";export default defineConfig({plugins:lazyPlugins(()=>[sveltekit({compilerOptions:{runes:({filename})=>filename.split(/[/\\]/).includes("node_modules")?undefined:true}})])});`,
		`export default {plugins:lazyPlugins(async()=>[sveltekit()])};`,
		"const base={server:{strictPort:true}};export default {...base, plugins:[]};",
		"export default { \"plugins\": [], server:{strictPort:true} };",
		"// plugins: [] is an authored comment.\nexport default {plugins:[], define:{text:'plugins: []'}};",
	} {
		result, err := viteConfig(source)
		if err != nil || !strings.Contains(result, "skgoTailwindcss()") {
			t.Fatalf("static config rejected: %s %v", source, err)
		}
		again, err := viteConfig(result)
		if err != nil || again != result {
			t.Fatalf("static config repeated: %s %v", again, err)
		}
	}
	for _, source := range []string{
		"const base={};export default {plugins:[], ...base};",
		"export default {plugins:[], plugins:[]};",
		"const plugins=[];export default {plugins};",
		`export default {["plugins"]:[]};`,
		"export default {plugins:[",
	} {
		if _, err := viteConfig(source); err == nil {
			t.Fatalf("ambiguous config accepted: %s", source)
		}
	}
}

func TestTailwindQualifiesExistingVersionsAndPreservesCharset(t *testing.T) {
	t.Parallel()
	project, p, _ := fixture(t, map[string]string{
		"package.json":              `{"name":"existing","dependencies":{"tailwindcss":"4.0.0","application-dependency":"1.2.3"},"devDependencies":{"@tailwindcss/vite":"^4.1.0","vite-plus":"1.3.0"}}`,
		"src/routes/+layout.svelte": "<script>import '../app.css';let {children}=$props();</script>{@render children()}",
		"src/app.css":               "@charset \"UTF-8\";\n.authored {color: rgb(17, 34, 51)}\n",
	})
	if _, err := p.Apply(context.Background(), templateapi.Request{Addon: "tailwindcss", Project: project}); err != nil {
		t.Fatal(err)
	}
	var pkg struct{ Dependencies, DevDependencies map[string]string }
	if err := json.Unmarshal([]byte(read(t, filepath.Join(project.Web, "package.json"))), &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Dependencies["tailwindcss"] != "4.3.3" || pkg.DevDependencies["@tailwindcss/vite"] != "4.3.3" || pkg.Dependencies["application-dependency"] != "1.2.3" {
		t.Fatalf("version qualification: %+v", pkg)
	}
	css := read(t, filepath.Join(project.Web, "src/app.css"))
	if !strings.HasPrefix(css, "@charset \"UTF-8\";\n@import \"tailwindcss\";") || !strings.Contains(css, ".authored {color: rgb(17, 34, 51)}") {
		t.Fatalf("charset or authored CSS lost: %s", css)
	}
}

func TestTailwindPreservesAuthoredPackageStringsAndIndentation(t *testing.T) {
	t.Parallel()
	for _, indent := range []string{"\t", "    "} {
		t.Run(fmt.Sprintf("indent-%q", indent), func(t *testing.T) {
			t.Parallel()
			source := `{"name":"authored","scripts":{"preview":"cd .. && just serve","markup":"echo '<authored> && <kept>'"},"dependencies":{"authored":"git+https://example.test/repo?a=1&b=2"},"devDependencies":{"vite-plus":"1.3.0"}}`
			var formatted bytes.Buffer
			encoder := json.NewEncoder(&formatted)
			encoder.SetEscapeHTML(false)
			encoder.SetIndent("", indent)
			var pkg map[string]any
			if err := json.Unmarshal([]byte(source), &pkg); err != nil {
				t.Fatal(err)
			}
			if err := encoder.Encode(pkg); err != nil {
				t.Fatal(err)
			}
			project, p, _ := fixture(t, map[string]string{"package.json": formatted.String()})
			if _, err := p.Apply(context.Background(), templateapi.Request{Addon: "tailwindcss", Project: project}); err != nil {
				t.Fatal(err)
			}
			after := read(t, filepath.Join(project.Web, "package.json"))
			for _, literal := range []string{`"preview": "cd .. && just serve"`, `"markup": "echo '<authored> && <kept>'"`, `"authored": "git+https://example.test/repo?a=1&b=2"`} {
				if !strings.Contains(after, literal) {
					t.Fatalf("authored JSON string changed: %s", after)
				}
			}
			if !strings.Contains(after, "\n"+indent+`"scripts": {`+"\n"+indent+indent) || !strings.HasSuffix(after, "\n") {
				t.Fatalf("authored indentation changed: %q", after)
			}
		})
	}
}
