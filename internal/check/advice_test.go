package check

import "testing"

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
