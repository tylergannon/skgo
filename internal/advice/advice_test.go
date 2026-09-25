package advice

import (
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAdvice(t *testing.T) {
	results := analysistest.Run(t, analysistest.TestData(), Analyzer, "a", "a/links/onzggl3sn52xizltf5nwg5ltorxw2zlsjfcf2")
	seen := make(map[string]bool)
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			seen[diagnostic.Category] = true
			if diagnostic.End <= diagnostic.Pos {
				t.Errorf("%s has no source range", diagnostic.Category)
			}
		}
	}
	for _, code := range []string{QueryCookie, QueryPageInput, RequestContext, OperationError, RefreshKind, FormField, RouteParam} {
		if !seen[code] {
			t.Errorf("missing diagnostic category %s", code)
		}
	}
}

func TestQueryAdvice(t *testing.T) {
	results := analysistest.Run(t, analysistest.TestData(), Analyzer, "q")
	counts := map[string]int{}
	for _, result := range results {
		for _, diagnostic := range result.Diagnostics {
			counts[diagnostic.Category]++
			if diagnostic.End <= diagnostic.Pos {
				t.Errorf("%s has no authored source range", diagnostic.Category)
			}
		}
	}
	if counts[QueryCookie] != 4 || counts[QueryPageInput] != 6 || len(counts) != 2 {
		t.Fatalf("query fixture diagnostics = %v; want four cookie writes and six page reads only", counts)
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

func TestRefreshAndRouteAdvice(t *testing.T) {
	results := analysistest.Run(t, analysistest.TestData(), Analyzer,
		"r",
		"a/links/onzggl3sn52xizltf5nwg5ltorxw2zlsjfcf2",
		"a/links/onzggl3sn52xizltf5nwg5ltorxw2zlsjfcf2l23ln2gcys5luxvwlrofz2gc2lmlu",
	)
	counts := map[string]int{}
	for _, result := range results {
		for _, d := range result.Diagnostics {
			if d.Category != RefreshKind && d.Category != RouteParam {
				continue
			}
			counts[d.Category]++
			if d.End <= d.Pos {
				t.Errorf("%s has no authored source range", d.Category)
			}
		}
	}
	if counts[RefreshKind] != 6 || counts[RouteParam] != 2 {
		t.Fatalf("refresh/route findings = %v; want six target mismatches and two absent route parameters", counts)
	}
}
