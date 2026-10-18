package adapter

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// One app and three build outcomes qualify analyse dispatch, precedence and
// Kit's matcher refusal. All successful artifact assertions reuse its first build.
func TestGoRouteEntriesDuringKitAnalyse(t *testing.T) {
	fixture := prepareMinimalInputsApp(t)
	write := func(path, source string) { writeFixtureFile(t, filepath.Join(fixture, path), source) }
	write("web/src/params.ts", `import { defineParams } from '@sveltejs/kit/params'; export const params = defineParams({ Fruit: (value: string) => value === 'atlas' || value === 'beacon' ? value : undefined });`)
	write("web/src/params.go", `package hooks
type CatalogItem struct{Name string}
func Fruit(s string)(CatalogItem,bool){return CatalogItem{Name:s},s=="atlas"||s=="beacon"}
`)
	write("web/src/routes/catalog/[...rest]/+page.svelte", `<h1>Fallback route: rejected catalog entry</h1>`)
	write("web/src/routes/catalog/[slug=Fruit]/[[edition]]/+page.ts", `export const prerender = true;`)
	write("web/src/routes/catalog/[slug=Fruit]/[[edition]]/+page.svelte", `<script lang="ts">let { data } = $props();</script><h1>{data.receipt}</h1>`)
	write("web/src/routes/catalog/[slug=Fruit]/[[edition]]/page.server.go", `package catalog
import("fmt";"os";"github.com/tylergannon/skgo")
type Data struct{Receipt string `+"`json:\"receipt\"`"+`}
func load(e PageRequestEvent)(Data,error){version:="original";if e.Params.Edition()!=nil{version=*e.Params.Edition()};return Data{Receipt:"Go catalog: "+e.Params.Slug().Name+"|"+version},nil}
var _=skgo.Load(load)
func entries()([]EntryParams,error){switch os.Getenv("SKGO_ENTRIES_FAILURE"){case "producer":return nil,fmt.Errorf("catalog offline");case "matcher":return []EntryParams{{Slug:"rejected"}},nil};version:="v2";return []EntryParams{{Slug:"atlas"},{Slug:"beacon",Edition:&version}},nil}
var _=skgo.Entries(entries)
`)
	for _, route := range []string{"universal", "precedence"} {
		write("web/src/routes/"+route+"/[slug]/+page.ts", `export const prerender = true;`)
		write("web/src/routes/"+route+"/[slug]/+page.svelte", `<script lang="ts">let { data } = $props();</script><h1>{data.receipt}</h1>`)
		producer := `return []EntryParams{{Slug:"page"}},nil`
		if route == "universal" {
			producer = `panic("universal entries must override Go server entries")`
			write("web/src/routes/universal/[slug]/+page.ts", `export const prerender=true;export const entries=()=>[{slug:'universal'}];`)
		}
		write("web/src/routes/"+route+"/[slug]/page.server.go", `package route
import "github.com/tylergannon/skgo"
type Data struct{Receipt string `+"`json:\"receipt\"`"+`}
func load(e PageRequestEvent)(Data,error){return Data{Receipt:"Go selected: "+e.Params.Slug()},nil}
var _=skgo.Load(load)
func entries()([]EntryParams,error){`+producer+`}
var _=skgo.Entries(entries)
`)
	}
	write("web/src/routes/precedence/[slug]/server.go", `package route
import("net/http";"github.com/tylergannon/skgo")
func endpointEntries()([]EntryParams,error){panic("page entries must override endpoint entries")}
var _=skgo.Entries(endpointEntries)
func get(w http.ResponseWriter,r *http.Request){w.Write([]byte("Go endpoint"))}
var _=skgo.GET(get)
`)
	receipt := filepath.Join(fixture, "endpoint-entries-receipt")
	write("web/src/routes/endpoint/[slug]/server.go", `package endpoint
import("net/http";"os";"github.com/tylergannon/skgo")
func entries()([]EntryParams,error){if err:=os.WriteFile(os.Getenv("SKGO_ENTRIES_RECEIPT"),[]byte("Go endpoint entries: endpoint"),0600);err!=nil{return nil,err};return []EntryParams{{Slug:"endpoint"}},nil}
var _=skgo.Entries(entries)
func get(w http.ResponseWriter,r *http.Request){w.Write([]byte("Go endpoint"))}
var _=skgo.GET(get)
`)
	generate := exec.Command("go", "generate", "./internal/skgo")
	generate.Dir = fixture
	generate.Env = append(fixtureBuildEnv(), "GOWORK=off")
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate: %v\n%s", err, output)
	}
	if _, err := os.Stat(receipt); !os.IsNotExist(err) {
		t.Fatalf("entries ran before Kit analyse: %v", err)
	}
	for _, mode := range []string{"success", "producer", "matcher"} {
		t.Run(mode, func(t *testing.T) {
			build := exec.Command(filepath.Join(fixture, "web/node_modules/.bin/vp"), "build")
			build.Dir = filepath.Join(fixture, "web")
			build.Env = append(fixtureBuildEnv(), "SKGO_ENTRIES_RECEIPT="+receipt, "SKGO_ENTRIES_FAILURE="+mode)
			output, err := build.CombinedOutput()
			t.Logf("vp build mode=%q:\n%s", mode, output)
			if mode == "success" {
				if err != nil {
					t.Fatalf("build: %v", err)
				}
				for path, want := range map[string]string{"catalog/atlas.html": "Go catalog: atlas|original", "catalog/beacon/v2.html": "Go catalog: beacon|v2", "universal/universal.html": "Go selected: universal", "precedence/page.html": "Go selected: page"} {
					data, readErr := os.ReadFile(filepath.Join(fixture, "web/build/prerendered", path))
					if readErr != nil || !strings.Contains(string(data), want) {
						t.Fatalf("artifact %s: err=%v missing literal %q", path, readErr, want)
					}
				}
				data, readErr := os.ReadFile(receipt)
				if readErr != nil || string(data) != "Go endpoint entries: endpoint" {
					t.Fatalf("endpoint analyse receipt=%q err=%v", data, readErr)
				}
				for _, path := range []string{"catalog/atlas/__data.json", "catalog/beacon/v2/__data.json"} {
					if _, err := os.Stat(filepath.Join(fixture, "web/build/prerendered", path)); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := os.Stat(filepath.Join(fixture, "web/build/prerendered/catalog/unlisted.html")); !os.IsNotExist(err) {
					t.Fatalf("unlisted artifact: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("invalid entries built successfully")
				}
				for _, want := range []string{"/catalog/[slug=Fruit]/[[edition]]"} {
					if !strings.Contains(string(output), want) {
						t.Fatalf("failure did not name route %q", want)
					}
				}
				want := "catalog offline"
				if mode == "matcher" {
					want = "rejected"
				}
				if !strings.Contains(string(output), want) {
					t.Fatalf("failure missing %q", want)
				}
			}
		})
	}
}
