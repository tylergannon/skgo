package check

import (
	"errors"
	"testing"
)

func TestSkgoDuplicateRetainsBothAuthoredLocations(t *testing.T) {
	root := t.TempDir()
	msg := "skgo: src/routes/todos/todos.remote.ts#get is declared twice, at " + root + "/web/src/routes/todos/a.remote.go:8:9 and " + root + "/web/src/routes/todos/b.remote.go:13:4"
	d := diagnosticFromError("skgo", skgoCode(msg), root, msg)
	if d.Code != "duplicate-declaration" || d.Location == nil || d.Location.File != "web/src/routes/todos/a.remote.go" || len(d.Related) != 1 || d.Related[0].File != "web/src/routes/todos/b.remote.go" {
		t.Fatalf("diagnostic=%+v", d)
	}
}

func TestGoToolZeroExitFailureTextIsIncomplete(t *testing.T) {
	c, ds := checkGoOutput("go-build", t.TempDir(), routeInventory{}, []byte("build failed but exited zero\n"), nil)
	if c.Status != "incomplete" || len(ds) != 0 {
		t.Fatalf("check=%+v diagnostics=%+v", c, ds)
	}
}

func TestGoTypeErrorStillProducesAuthoredDiagnostic(t *testing.T) {
	root := t.TempDir()
	c, ds := checkGoOutput("go-build", root, routeInventory{}, []byte("# example.com/app\n./main.go:3:9: cannot use string as int\n"), errors.New("exit status 1"))
	if c.Status != "complete" || len(ds) != 1 || ds[0].Location == nil || ds[0].Location.File != "main.go" || ds[0].Location.Line != 3 {
		t.Fatalf("check=%+v diagnostics=%+v", c, ds)
	}
}

func TestStaticcheckCompileFailureIsIncomplete(t *testing.T) {
	output := []byte(`{"code":"compile","severity":"error","location":{},"message":"Go type error"}`)
	c, ds := checkStaticcheck(t.TempDir(), routeInventory{}, output, errors.New("exit status 1"))
	if c.Status != "incomplete" || len(ds) != 0 {
		t.Fatalf("check=%+v diagnostics=%+v", c, ds)
	}
}

func TestWireContractDiagnosticCode(t *testing.T) {
	for _, message := range []string{
		"result cannot cross to TypeScript",
		"skgo.File returns from a query",
		"skgo.Deferred is only valid in a load",
		"duplicate serialized name on wire",
	} {
		if got := skgoCode(message); got != "SKGO007" {
			t.Errorf("%q has code %q", message, got)
		}
	}
}

func TestStaleRouteLinkHasAuthoredFileWithoutFabricatedRange(t *testing.T) {
	root := t.TempDir()
	message := "skgo: " + root + "/web/src/routes/todos/todos.remote.go: generated route link is stale"
	d := diagnosticFromError("skgo", skgoCode(message), root, message)
	if d.Location == nil || d.Location.File != "web/src/routes/todos/todos.remote.go" || d.Location.Line != 0 {
		t.Fatalf("diagnostic=%+v", d)
	}
}

func TestDuplicateSerializedNameRetainsFieldAndDeclaration(t *testing.T) {
	root := t.TempDir()
	message := "skgo: " + root + "/web/src/data.remote.go:17:4: read: " + root + "/web/src/data.remote.go:12:2: duplicate serialized name"
	d := skgoDiagnostic(root, message)
	if d.Code != "SKGO007" || d.Documentation != "skgo advice SKGO007" || d.Location == nil || d.Location.Line != 12 || len(d.Related) != 1 || d.Related[0].Line != 17 {
		t.Fatalf("diagnostic=%+v", d)
	}
}
