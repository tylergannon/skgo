package skgo

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

type fixtureNumber int64

func (n fixtureNumber) Label() string { return fmt.Sprintf("number:%d", n) }

// Patterns, route metadata, paths and answers are literal fixtures mapped from
// Kit 3.0.0's parse_route_id/exec. No expectation reads the router's output.
func TestTypedLoadMatcherRouting(t *testing.T) {
	matchers := map[string]ParamMatcher{
		"Number": func(s string) (any, bool) {
			n, err := strconv.ParseInt(s, 10, 64)
			return fixtureNumber(n), err == nil && n >= 0
		},
		"Flag":  func(s string) (any, bool) { return s == "true", s == "true" || s == "false" },
		"Empty": func(s string) (any, bool) { return "", s == "blank" || s == "" },
		"Alpha": func(s string) (any, bool) { return s, s == "a" },
	}
	type routeFixture struct {
		id, pattern, evidence string
		params                []ManifestParam
	}
	fixtures := []routeFixture{
		{"/value/special", `^\/value\/special\/?$`, "literal", nil},
		{"/value/42", `^\/value\/42\/?$`, "literal-number", nil},
		{"/value/[n=Number]", `^\/value\/([^/]+?)\/?$`, "number", []ManifestParam{{Name: "n", Matcher: "Number"}}},
		{"/value/[text]", `^\/value\/([^/]+?)\/?$`, "fallback", []ManifestParam{{Name: "text"}}},
		{"/only/[n=Number]", `^\/only\/([^/]+?)\/?$`, "number", []ManifestParam{{Name: "n", Matcher: "Number"}}},
		{"/flag/[flag=Flag]", `^\/flag\/([^/]+?)\/?$`, "flag", []ManifestParam{{Name: "flag", Matcher: "Flag"}}},
		{"/empty/[...text=Empty]", `^\/empty(?:\/([^]*))?\/?$`, "empty", []ManifestParam{{Name: "text", Matcher: "Empty", Rest: true, Chained: true}}},
		{"/chain/[[a=Alpha]]/[[b=Number]]/[...rest]", `^\/chain(?:\/([^/]+))?(?:\/([^/]+))?(?:\/([^]*))?\/?$`, "chain", []ManifestParam{{Name: "a", Matcher: "Alpha", Optional: true, Chained: true}, {Name: "b", Matcher: "Number", Optional: true, Chained: true}, {Name: "rest", Rest: true, Chained: true}}},
		{"/shift/[[a=Alpha]]/[[b=Number]]", `^\/shift(?:\/([^/]+))?(?:\/([^/]+))?\/?$`, "shift", []ManifestParam{{Name: "a", Matcher: "Alpha", Optional: true, Chained: true}, {Name: "b", Matcher: "Number", Optional: true, Chained: true}}},
		{"/embedded/pre[[a=Alpha]]post/[...rest]", `^\/embedded\/pre([^/]*)?post(?:\/([^]*))?\/?$`, "embedded", []ManifestParam{{Name: "a", Matcher: "Alpha", Optional: true}, {Name: "rest", Rest: true, Chained: true}}},
	}
	cfg := LoadConfig{Origin: "http://127.0.0.1:8080"}
	var loads []*ServerLoad
	for i, fixture := range fixtures {
		module := "src/routes" + fixture.id + "/+page.server.ts"
		cfg.Nodes = append(cfg.Nodes, module)
		cfg.Routes = append(cfg.Routes, ManifestRoute{ID: fixture.id, Pattern: fixture.pattern, Params: fixture.params, Page: &ManifestPage{Leaf: i}})
		loads = append(loads, NewServerLoad(LoadSpec{Module: module, Matchers: matchers, Run: func(ctx context.Context) (any, error) {
			e := EventFrom(ctx)
			answer := fixture.evidence
			switch fixture.evidence {
			case "number":
				typed := RequestEvent[struct{ N fixtureNumber }]{Event: e, Params: struct{ N fixtureNumber }{LoadParamValue[fixtureNumber](e, "n")}}
				answer = typed.Params.N.Label() + "|skgo.fixtureNumber|" + fixture.id
			case "fallback":
				answer += ":" + LoadParamValue[string](e, "text") + "|" + fixture.id
			case "flag":
				answer += fmt.Sprintf(":%t|bool", LoadParamValue[bool](e, "flag"))
			case "empty":
				answer += ":" + LoadParamValue[string](e, "text") + "|string"
			case "chain", "shift", "embedded":
				a, b := OptionalLoadParamValue[string](e, "a"), OptionalLoadParamValue[fixtureNumber](e, "b")
				alpha, number := "absent", "absent"
				if a != nil {
					alpha = *a
				}
				if b != nil {
					number = b.Label()
				}
				answer += ":" + alpha + "|" + number + "|" + LoadParamValue[string](e, "rest")
			}
			return struct {
				Evidence string `json:"evidence"`
			}{answer}, nil
		}}))
	}
	handler, err := NewLoads(cfg, loads...)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, evidence string
		status         int
	}{
		{"/value/special", "literal", 200},
		{"/value/42", "literal-number", 200},
		{"/value/00042", "number:42|skgo.fixtureNumber|/value/[n=Number]", 200},
		{"/value/0", "number:0|skgo.fixtureNumber|/value/[n=Number]", 200},
		{"/value/no", "fallback:no|/value/[text]", 200},
		{"/value/9223372036854775808", "fallback:9223372036854775808|/value/[text]", 200},
		{"/only/no", "", 404},
		{"/only/-1", "", 404},
		{"/flag/false", "flag:false|bool", 200},
		{"/flag/true", "flag:true|bool", 200},
		{"/flag/no", "", 404},
		{"/empty/blank", "empty:|string", 200},
		{"/empty", "empty:|string", 200},
		{"/empty/no", "", 404},
		{"/chain/a/7/tail", "chain:a|number:7|tail", 200},
		{"/chain/7/tail/more", "chain:absent|number:7|tail/more", 200},
		{"/chain/no/7/tail", "chain:absent|absent|no/7/tail", 200},
		{"/chain", "chain:absent|absent|", 200},
		{"/shift/7", "shift:absent|number:7|", 200},
		{"/shift/7/tail", "", 404},
		{"/embedded/pre7post/tail", "", 404},
	} {
		t.Run(tc.path, func(t *testing.T) {
			response := get(t, handler, tc.path+"/__data.json")
			if response.Code != tc.status {
				t.Fatalf("status %d, want %d; %s", response.Code, tc.status, response.Body.String())
			}
			if tc.status == http.StatusOK {
				// Decode only the response envelope. The evidence itself is an
				// independent literal, including the named method's return value.
				want := `{"evidence":1},"` + tc.evidence + `"`
				if !strings.Contains(response.Body.String(), want) {
					t.Fatalf("body %s lacks literal %s", response.Body.String(), want)
				}
			}
		})
	}
}

// A nil declared interface is present only when the selected route captured it.
// These assertions exercise the real handler and require the load to execute.
type fixtureRef interface{ Label() string }
type fixtureConcreteRef int

func (r fixtureConcreteRef) Label() string { return fmt.Sprintf("Ref #%d", r) }

func TestTypedLoadNilInterfaceBoundary(t *testing.T) {
	matcher := func(s string) (any, bool) {
		if s == "none" {
			var ref fixtureRef
			return ref, true
		}
		if s == "42" {
			return fixtureConcreteRef(42), true
		}
		return nil, false
	}
	cfg := LoadConfig{Origin: "http://127.0.0.1:8080", Nodes: []string{"required", "optional"}, Routes: []ManifestRoute{
		{ID: "/nil/[id=MaybeRef]", Pattern: "^/nil/([^/]+?)/?$", Params: []ManifestParam{{Name: "id", Matcher: "MaybeRef"}}, Page: &ManifestPage{Leaf: 0}},
		{ID: "/nil-optional/[[id=MaybeRef]]", Pattern: "^/nil-optional(?:/([^/]+))?/?$", Params: []ManifestParam{{Name: "id", Matcher: "MaybeRef", Optional: true, Chained: true}}, Page: &ManifestPage{Leaf: 1}},
	}}
	calls := 0
	load := func(optional bool) func(context.Context) (any, error) {
		return func(ctx context.Context) (any, error) {
			calls++
			event := EventFrom(ctx)
			label := "load:present:nil"
			var ref fixtureRef
			if optional {
				ptr := OptionalLoadParamValue[fixtureRef](event, "id")
				if ptr == nil {
					label = "load:absent"
				} else {
					ref = *ptr
				}
			} else {
				ref = LoadParamValue[fixtureRef](event, "id")
			}
			TrackLoadParam(event, "id")
			if ref != nil {
				label = "load:present:" + ref.Label()
			}
			return struct {
				Receipt string `json:"receipt"`
			}{label}, nil
		}
	}
	handler, err := NewLoads(cfg,
		NewServerLoad(LoadSpec{Module: "required", Matchers: map[string]ParamMatcher{"MaybeRef": matcher}, Run: load(false)}),
		NewServerLoad(LoadSpec{Module: "optional", Matchers: map[string]ParamMatcher{"MaybeRef": matcher}, Run: load(true)}),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, want string }{
		{"/nil/none/__data.json", "load:present:nil"},
		{"/nil-optional/none/__data.json", "load:present:nil"},
		{"/nil-optional/__data.json", "load:absent"},
		{"/nil/42/__data.json", "load:present:Ref #42"},
		{"/nil-optional/42/__data.json", "load:present:Ref #42"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			before := calls
			response := get(t, handler, tc.path)
			if response.Code != 200 || !strings.Contains(response.Body.String(), `{"receipt":1},"`+tc.want+`"`) || !strings.Contains(response.Body.String(), `"params":["id"]`) || calls != before+1 {
				t.Fatalf("want executed load %q and id dependency; %d %s (calls %d)", tc.want, response.Code, response.Body.String(), calls-before)
			}
		})
	}
}

func TestTypedLoadNilMismatches(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(*Event)
	}{
		{"int", func(e *Event) { LoadParamValue[int](e, "id") }},
		{"pointer", func(e *Event) { LoadParamValue[*fixtureConcreteRef](e, "id") }},
		{"optional-int", func(e *Event) { OptionalLoadParamValue[int](e, "id") }},
		{"wrong-interface", func(e *Event) { LoadParamValue[fixtureRef](e, "wrong") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &Event{load: &loadState{shared: &loadRequest{params: map[string]string{"id": "none", "wrong": "42"}, converted: map[string]any{"id": nil, "wrong": 42}}}}
			defer func() {
				if recover() == nil {
					t.Fatal("mismatched conversion silently accepted")
				}
			}()
			tc.run(e)
		})
	}
}
