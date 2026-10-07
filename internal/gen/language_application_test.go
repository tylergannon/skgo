package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedLanguageApplications(t *testing.T) {
	t.Parallel()
	const transport = `package hooks
import "github.com/tylergannon/skgo"
type Money struct { Cents int ` + "`json:\"cents\"`" + ` }
var _=skgo.Transported[Money]("Money")
`
	const price = `package routes
import (hooks "example.com/app/web/src";"github.com/tylergannon/skgo")
type PageData struct { Price hooks.Money ` + "`json:\"price\"`" + ` }
func page(PageRequestEvent)(PageData,error){return PageData{},nil}
var _=skgo.Load(page)
`
	root, cfg := foreignFixture(t, jsFixtureRemote, map[string]string{
		"app/web/src/routes/account/page.server.go": javaScriptLoadActionFixture,
		"app/web/src/routes/about/page.server.go":   strings.ReplaceAll(loadSource, "LayoutRequestEvent", "PageRequestEvent"),
		"app/web/src/routes/about/+page.ts":         "export const prerender=true;\n",
		"app/web/src/hooks.go":                      transport,
		"app/web/src/routes/price/page.server.go":   price,
	})
	cfg.Language = LanguageTypeScript
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	t.Run("TypeScriptLoadBridge", func(t *testing.T) { testTypeScriptLoadBridge(t, root) })
	for _, name := range []string{"data.remote.ts", "types.ts"} {
		if _, err := os.Stat(filepath.Join(cfg.Web, "src/data", name)); err != nil {
			t.Fatalf("missing TypeScript prerequisite %s: %v", name, err)
		}
	}
	authored := filepath.Join(cfg.Web, "src/data/handwritten.js")
	const sentinel = "export const mine = 1;\n"
	if err := os.WriteFile(authored, []byte(sentinel), 0644); err != nil {
		t.Fatal(err)
	}
	writeSharedFixture(t, cfg.Web, "jsconfig.json", "{\"extends\":\"$app/tsconfig\"}\n")
	cfg.Language = LanguageAuto
	if err := Run(cfg); err != nil {
		t.Fatalf("auto JavaScript generation: %v", err)
	}
	t.Run("JavaScriptRemotesAndTypes", func(t *testing.T) { testJavaScriptRemotesAndTypes(t, root) })
	t.Run("JavaScriptLoadActions", func(t *testing.T) { testJavaScriptLoadActions(t, root) })
	t.Run("JavaScriptTransportedLoad", func(t *testing.T) { testJavaScriptTransportedLoad(t, root) })
	stub := readFixtureFile(t, root, "app/web/src/routes/about/+page.server.js")
	if !strings.Contains(stub, `buildLoad("src/routes/about/+page.server.js", "src/routes/about/page.server.go", event)`) {
		t.Fatalf("prerendered JavaScript load lost its build bridge: %s", stub)
	}
	if got, err := os.ReadFile(authored); err != nil || string(got) != sentinel {
		t.Fatalf("language switch changed authored file: %s %v", got, err)
	}
}
