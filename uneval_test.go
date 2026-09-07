package skgo

import "testing"

// TestUnevalWritesEvaluableJavaScript pins the output devalue's own `uneval`
// produces for the shapes a load or a remote function can return, because the
// browser evaluates this text rather than parsing it. The escaping is the half
// that matters: a value carrying `</script>` would otherwise close the element
// it is written into.
func TestUnevalWritesEvaluableJavaScript(t *testing.T) {
	cases := map[string]struct {
		value any
		want  string
	}{
		"null":           {nil, "null"},
		"true":           {true, "true"},
		"string":         {"ada", `"ada"`},
		"integer":        {float64(42), "42"},
		"fraction":       {0.5, ".5"},
		"negative":       {-0.25, "-.25"},
		"empty array":    {[]any{}, "[]"},
		"array":          {[]any{"a", float64(1), nil}, `["a",1,null]`},
		"object":         {map[string]any{"name": "skgo"}, `{name:"skgo"}`},
		"sorted keys":    {map[string]any{"b": float64(2), "a": float64(1)}, `{a:1,b:2}`},
		"quoted key":     {map[string]any{"a-b": float64(1)}, `{"a-b":1}`},
		"reserved key":   {map[string]any{"class": float64(1)}, `{"class":1}`},
		"nested":         {map[string]any{"a": map[string]any{"b": []any{float64(1)}}}, `{a:{b:[1]}}`},
		"newline":        {"a\nb", `"a\nb"`},
		"quote":          {`he said "no"`, `"he said \"no\""`},
		"backslash":      {`a\b`, `"a\\b"`},
		"line separator": {"a\u2028b", `"a\u2028b"`},
		"control byte":   {"a\x01b", `"a\u0001b"`},
		"closing a tag":  {"</script>", `"\u003C/script>"`},
		"less than":      {"1 < 2", `"1 \u003C 2"`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := uneval(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("uneval(%#v) = %s, want %s", tc.value, got, tc.want)
			}
		})
	}
}

// TestUnevalRefusesWhatCannotTravel keeps a value the browser cannot evaluate
// from being written into a script element as whatever Go printed for it.
func TestUnevalRefusesWhatCannotTravel(t *testing.T) {
	for name, value := range map[string]any{
		"a function": func() {},
		"a channel":  make(chan int),
		"a struct":   struct{ A int }{1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := uneval(value); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}
