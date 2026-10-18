package advice

import (
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAdvice(t *testing.T) {
	const route = "a/links/onzggl3sn52xizltf5nwg5ltorxw2zlsjfcf2"
	const nestedRoute = "a/links/onzggl3sn52xizltf5nwg5ltorxw2zlsjfcf2l23ln2gcys5luxvwlrofz2gc2lmlu"
	// One package load checks every registered-handler fixture. analysistest
	// matches messages at their authored locations; these literal counts also
	// pin the SKGO rule identifiers exposed to CLI and editor consumers.
	want := map[string]map[string]int{
		"a":         {QueryCookie: 3, QueryPageInput: 5, RequestContext: 2, OperationError: 9, RefreshKind: 2, FormField: 5},
		"q":         {QueryCookie: 4, QueryPageInput: 6},
		"r":         {RefreshKind: 6},
		route:       {RouteParam: 1},
		nestedRoute: {RouteParam: 1},
	}
	results := analysistest.Run(t, analysistest.TestData(), Analyzer, "a", "q", "r", route, nestedRoute)
	if len(results) != len(want) {
		t.Fatalf("analyzed %d fixture packages, want %d", len(results), len(want))
	}
	for _, result := range results {
		counts := make(map[string]int)
		for _, diagnostic := range result.Diagnostics {
			counts[diagnostic.Category]++
			if diagnostic.End <= diagnostic.Pos {
				t.Errorf("%s has no source range", diagnostic.Category)
			}
		}
		path := result.Pass.Pkg.Path()
		if !reflect.DeepEqual(counts, want[path]) {
			t.Errorf("%s diagnostics = %v, want %v", path, counts, want[path])
		}
	}
}

func TestRouteKnowledge(t *testing.T) {
	page := filepath.Join("app", "src", "routes", "[customerID]", "[[tab]]", "[...tail]", "page.server.go")
	got := routeParams(page)
	for _, name := range []string{"customerID", "tab", "tail"} {
		if !got[name] {
			t.Errorf("missing %s", name)
		}
	}
	if routeParams(filepath.Join("app", "src", "routes", "[id]", "layout.server.go")) != nil {
		t.Fatal("shared layout was treated as a single known route")
	}
	linked := filepath.Join("app", "internal", "skgo", "links", "onzggl3sn52xizltf5nwg5ltorxw2zlsjfcf2", "page.server.go")
	if !routeParams(linked)["customerID"] {
		t.Fatal("generated route link did not recover the authored parameter")
	}
	matched := filepath.Join("app", "src", "routes", "[id=integer]", "page.server.go")
	if !routeParams(matched)["id"] {
		t.Fatal("matched route parameter was not recognized")
	}
	if routeParams(filepath.Join("app", "src", "routes", "prefix-[id]", "page.server.go")) != nil {
		t.Fatal("mixed route segment was asserted with incomplete route knowledge")
	}
}
