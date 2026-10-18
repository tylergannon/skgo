package render

import (
	"strings"
	"testing"
)

func TestSlugifyMatchesTheSite(t *testing.T) {
	cases := map[string]string{
		"Universal vs server":    "Universal-vs-server",
		"Using `fetch`":          "Using-fetch",
		"Invalid.Add":            "Invalid.Add",
		"What’s next":            "What's-next",
		"Load[P, L any]":         "Load-P-L-any",
		"A heading ... ellipsis": "A-heading-ellipsis",
	}
	for in, want := range cases {
		if got := slugify(plainText(in)); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHeadingIDsNest(t *testing.T) {
	ids := headingIDs("## Invalid\n\ntext\n\n### Add\n\n```go\n## not a heading\n```\n\n## Issue\n\n### Add\n")
	for _, want := range []string{"Invalid", "Invalid-Add", "Issue", "Issue-Add"} {
		if !ids[want] {
			t.Errorf("missing id %q in %v", want, ids)
		}
	}
	if ids["not-a-heading"] {
		t.Error("a heading inside a fence counted")
	}
}

func TestDedentKeepsRelativeIndentation(t *testing.T) {
	got := dedent([]string{"\tfunc f() {", "\t\treturn", "", "\t}"})
	want := "func f() {\n\treturn\n\n}"
	if got != want {
		t.Errorf("dedent = %q, want %q", got, want)
	}
}

func TestExpandIncludesRequiresRealGo(t *testing.T) {
	page := &Page{File: "p.md", bodyLine: 1, Body: "```go\n/// file: x.go\nfunc typed() {}\n```\n"}
	_, problems := expandIncludes(t.TempDir(), page)
	if len(problems) != 1 || !strings.Contains(problems[0].Message, "include") {
		t.Fatalf("problems = %v", problems)
	}
	page = &Page{File: "p.md", bodyLine: 1, Body: "```sh\nskgo new app\n```\n"}
	if _, problems := expandIncludes(t.TempDir(), page); len(problems) != 0 {
		t.Fatalf("typed sh fence flagged: %v", problems)
	}
}

func TestCommandTableRendersFlags(t *testing.T) {
	text, err := commandTable("generate")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"```sh\nskgo generate [flags]\n```", "| `--web` | `../web` |", "| `--quiet` | `false` |"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	if _, err := commandTable("_plugin-probe"); err == nil {
		t.Error("internal command rendered")
	}
}
