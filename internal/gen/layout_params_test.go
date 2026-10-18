package gen

import (
	"go/types"
	"strings"
	"testing"

	"github.com/tylergannon/skgo"
)

func TestLoadParameterDomains(t *testing.T) {
	t.Parallel()
	matchers := map[string]goParamMatcher{"Number": {out: types.Typ[types.Int]}}
	params := []skgo.ManifestParam{{Name: "id", Matcher: "Number"}, {Name: "slug", Optional: true}}
	cfg := Config{LocalsType: "Locals"}
	imports := &fileImports{byPkg: map[*types.Package]string{}, used: map[string]bool{}}
	var page strings.Builder
	if err := writeConcreteLoadDomain(&page, imports, cfg, "RouteParams", "Page", params, matchers); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`func (p RouteParams) ID() int {skgo.TrackLoadParam(p.event,"id");return p.valueID}`,
		`func (p RouteParams) Slug() *string {skgo.TrackLoadParam(p.event,"slug");return p.valueSlug}`,
		`p.valueID = skgo.LoadParamValue[int](event,"id")`,
		`p.valueSlug = skgo.OptionalLoadParamValue[string](event,"slug")`,
		`type PageRequestEvent = skgo.RequestEvent[RouteParams, appstate.Locals]`,
	} {
		if !strings.Contains(page.String(), want) {
			t.Fatalf("page domain lacks %q:\n%s", want, page.String())
		}
	}
	// Kit supplies participating pages; SKGO widens only their converted types.
	var layout strings.Builder
	if err := writeLayoutDomain(&layout, imports, cfg, &routeLoadParams{
		params: []skgo.ManifestParam{{Name: "org"}},
		children: map[string]*routeLoadParams{
			"/number/[id=Number]": {params: params, matchers: matchers},
			"/text/[id]":          {params: []skgo.ManifestParam{{Name: "id"}}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`func (p LayoutParams) Org() string {skgo.TrackLoadParam(p.event,"org");return p.valueOrg}`,
		`func (p LayoutParams) Slug() *string {skgo.TrackLoadParam(p.event,"slug");return p.valueSlug}`,
		`type Key_ID interface { skgoParam_6964() }`,
		`type IDParam_Int struct { Value int }`,
		`type IDParam_String struct { Value string }`,
		`case "/number/[id=Number]":`,
		`p.valueID = IDParam_Int{Value:*value}`,
		`p.valueID = IDParam_String{Value:*value}`,
		`type LayoutRequestEvent = skgo.RequestEvent[LayoutParams, appstate.Locals]`,
	} {
		if !strings.Contains(layout.String(), want) {
			t.Fatalf("layout domain lacks %q:\n%s", want, layout.String())
		}
	}
}
