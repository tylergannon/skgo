package check

import (
	"errors"
	"strings"
	"testing"
)

func TestAdviceJSONZeroExitStillReportsFinding(t *testing.T) {
	root := t.TempDir()
	input := `{"example.com/app":{"skgoadvice":[{"posn":"` + root + `/src/routes/a.remote.go:4:3","end":"` + root + `/src/routes/a.remote.go:4:12","message":"query cannot set cookies","category":"skgo-query-cookie"}]}}` + "\n{}\n"
	c, ds := parseAdvice(root, routeInventory{}, []byte(input), nil)
	if c.Status != "complete" || len(ds) != 1 || ds[0].Code != "skgo-query-cookie" || ds[0].Location == nil || ds[0].Location.File != "src/routes/a.remote.go" || ds[0].Location.EndColumn != 12 {
		t.Fatalf("check=%+v diagnostics=%+v", c, ds)
	}
	pc, partial := parseAdvice(root, routeInventory{}, []byte(input+"{"), nil)
	if pc.Status != "incomplete" || len(partial) != 1 {
		t.Fatalf("partial check=%+v diagnostics=%+v", pc, partial)
	}
}

func TestAdviceCompileFailureNamesTheOriginalDiagnostic(t *testing.T) {
	input := "# example.com/app/src\nsrc/hooks.go:17:2: \"example.com/app/businesslogic\" imported and not used\n" + strings.Repeat("{}\n", 90)
	c, ds := parseAdvice(t.TempDir(), routeInventory{}, []byte(input), errors.New("exit status 1"))
	want := "Go analysis stopped: exit status 1: src/hooks.go:17:2: \"example.com/app/businesslogic\" imported and not used"
	if c.Status != "failed" || len(ds) != 0 || c.Message != want {
		t.Fatalf("check=%+v diagnostics=%+v", c, ds)
	}
	c, _ = parseAdvice(t.TempDir(), routeInventory{}, []byte(input), nil)
	want = "Go analysis reported errors before all analyzers completed: src/hooks.go:17:2: \"example.com/app/businesslogic\" imported and not used"
	if c.Status != "incomplete" || c.Message != want {
		t.Fatalf("zero-exit check=%+v", c)
	}
}

func TestAdviceMalformedJSONIsBoundedIncomplete(t *testing.T) {
	c, _ := parseAdvice(t.TempDir(), routeInventory{}, []byte("{\n\"a\": nope\n}\n"+strings.Repeat("{}\n", 90)), nil)
	if c.Status != "incomplete" || !strings.HasPrefix(c.Message, "Go analysis returned incomplete JSON: invalid character 'o'") || strings.Contains(c.Message, "{}") {
		t.Fatalf("check=%+v", c)
	}
}
