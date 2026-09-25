package gen

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestReadOnlyCheckFindsCurrentWireFieldBeforeStaleLink(t *testing.T) {
	app := sandboxExample(t)
	web := filepath.Join(app, "web")
	out := filepath.Join(app, "internal", "skgo")
	target := filepath.Join(web, "src", "routes", "todos", "todos.remote.go")
	original, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	const declaration = "type Rename struct {"
	if !bytes.Contains(original, []byte(declaration)) {
		t.Fatal("example no longer declares Rename; update the test")
	}
	broken := []byte(strings.Replace(string(original), declaration, declaration+"\n\tCallback func() `json:\"callback\"`", 1))
	if err := os.WriteFile(target, broken, 0o644); err != nil {
		t.Fatal(err)
	}
	links, err := filepath.Glob(filepath.Join(out, "links", "*", "todos.remote.go"))
	if err != nil || len(links) != 1 {
		t.Fatalf("expected one committed todos route copy, got %v: %v", links, err)
	}
	link := links[0]
	beforeLink, err := os.ReadFile(link)
	if err != nil {
		t.Fatal(err)
	}
	err = Check(Config{Web: web, Out: out})
	if err == nil || !strings.Contains(err.Error(), "field Callback cannot cross the wire") {
		t.Fatalf("current authored wire field was missed before stale-link check: %v", err)
	}
	if after, err := os.ReadFile(link); err != nil || !bytes.Equal(after, beforeLink) {
		t.Fatalf("generated route link changed: %v", err)
	}
	if after, err := os.ReadFile(target); err != nil || !bytes.Equal(after, broken) {
		t.Fatalf("authored file changed: %v", err)
	}
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Check(Config{Web: web, Out: out}); err != nil {
		t.Fatalf("valid counterpart did not check cleanly: %v", err)
	}
}

func TestRealCheckReportsWireAdviceAtAuthoredLocations(t *testing.T) {
	app := sandboxExample(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "skgo")
	build := exec.Command("go", "build", "-o", bin, "./cmd/skgo")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build skgo check: %v\n%s", err, output)
	}
	type diagnostic struct {
		Code, Message string
		Location      *struct {
			File string
			Line int
		}
	}
	type report struct {
		OK          bool
		Checks      []struct{ Name, Status string }
		Diagnostics []diagnostic
	}
	run := func(expectWireFailure bool) report {
		t.Helper()
		cmd := exec.Command(bin, "check", "--root", app, "--json")
		cmd.Dir = app
		output, runErr := cmd.CombinedOutput()
		var got report
		if err := json.Unmarshal(output, &got); err != nil {
			t.Fatalf("check returned incomplete JSON: %v; run=%v\n%s", err, runErr, output)
		}
		var exit *exec.ExitError
		if expectWireFailure {
			if !errors.As(runErr, &exit) || exit.ExitCode() != 1 || got.OK {
				t.Fatalf("check must retain the wire failure: run=%v report.OK=%v", runErr, got.OK)
			}
		} else if runErr == nil {
			if !got.OK {
				t.Fatal("check exited zero but reported a failure")
			}
		} else if !errors.As(runErr, &exit) || exit.ExitCode() != 1 || got.OK {
			t.Fatalf("check returned an inconsistent unrelated failure: run=%v report.OK=%v", runErr, got.OK)
		}
		return got
	}
	for _, tc := range []struct {
		file, anchor, insertion, field, reason, repair string
	}{
		{"todos/todos.remote.go", "type Rename struct {", "\n\tDetail []map[string]string `json:\"detail\"`", "Detail", "map", "named struct"},
		{"todos/todos.remote.go", "ID string `json:\"id\"`", "\n\tAlias string `json:\"id\"`", "Alias", "duplicate serialized name", "distinct json name"},
		{"todos/todos.remote.go", "type Rename struct {", "\n\tLater skgo.Deferred[string] `json:\"later\"`", "Later", "Deferred", "server load"},
		{"contact/contact.remote.go", "type Receipt struct {", "\n\tUpload skgo.File `json:\"upload\"`", "sendMessage", "skgo.File", "download URL"},
	} {
		t.Run(tc.field, func(t *testing.T) {
			path := filepath.Join(app, "web", "src", "routes", tc.file)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			planted := strings.Replace(string(original), tc.anchor, tc.anchor+tc.insertion, 1)
			if planted == string(original) {
				t.Fatal("fixture anchor missing")
			}
			wantLine := strings.Count(planted[:strings.Index(planted, tc.anchor)+len(tc.anchor)], "\n") + 2
			if err := os.WriteFile(path, []byte(planted), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.WriteFile(path, original, 0o644) })
			got := run(true)
			found := false
			for _, d := range got.Diagnostics {
				if d.Code == "SKGO007" && strings.Contains(d.Message, tc.reason) && strings.Contains(d.Message, tc.repair) && d.Location != nil && d.Location.File == filepath.ToSlash(filepath.Join("web", "src", "routes", tc.file)) && d.Location.Line == wantLine {
					found = true
				}
			}
			if !found {
				t.Fatalf("SKGO007 missing authored location and repair: %+v", got.Diagnostics)
			}
			if err := os.WriteFile(path, original, 0o644); err != nil {
				t.Fatal(err)
			}
		})
	}
	valid := run(false)
	for _, d := range valid.Diagnostics {
		if d.Code == "SKGO007" {
			t.Fatalf("supported counterpart reported wire advice: %+v", d)
		}
	}
	skgoComplete := false
	for _, c := range valid.Checks {
		if c.Name == "skgo" {
			skgoComplete = c.Status == "complete"
		}
	}
	if !skgoComplete {
		t.Fatalf("supported nested, array, upload and deferred values did not complete generator check: %+v", valid.Checks)
	}
}

func TestCheckWireAdviceUsesAuthoredDeclarations(t *testing.T) {
	app := sandboxExample(t)
	web := filepath.Join(app, "web")
	out := filepath.Join(app, "internal", "skgo")
	cfg := Config{Web: web, Out: out}
	todos := filepath.Join(web, "src", "routes", "todos", "todos.remote.go")
	contact := filepath.Join(web, "src", "routes", "contact", "contact.remote.go")
	for _, tc := range []struct {
		name, file, anchor, insertion, consequence, repair string
	}{
		{"nested unsupported", todos, "type Rename struct {", "\n\tDetail []map[string]string `json:\"detail\"`", "cannot cross the wire", "Use a named struct"},
		{"duplicate name", todos, "ID string `json:\"id\"`", "\n\tAlias string `json:\"id\"`", "duplicate serialized name", "distinct json name"},
		{"deferred remote", todos, "type Rename struct {", "\n\tLater skgo.Deferred[string] `json:\"later\"`", "Deferred", "server load"},
		{"file result", contact, "type Receipt struct {", "\n\tUpload skgo.File `json:\"upload\"`", "returns a skgo.File", "download URL"},
		{"deferred file result", filepath.Join(web, "src", "routes", "stream", "page.server.go"), "type PageData struct {", "\n\tUpload skgo.Deferred[skgo.File] `json:\"upload\"`", "returns a skgo.File", "download URL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(original, []byte(tc.anchor)) {
				t.Fatalf("missing fixture anchor %q", tc.anchor)
			}
			planted := strings.Replace(string(original), tc.anchor, tc.anchor+tc.insertion, 1)
			if err := os.WriteFile(tc.file, []byte(planted), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.WriteFile(tc.file, original, 0o644) })
			err = Check(cfg)
			if err == nil || !strings.Contains(err.Error(), tc.consequence) || !strings.Contains(err.Error(), tc.repair) || !strings.Contains(err.Error(), filepath.Base(tc.file)+":") {
				t.Fatalf("wanted authored wire advice with consequence and repair, got %v", err)
			}
			if err := os.WriteFile(tc.file, original, 0o644); err != nil {
				t.Fatal(err)
			}
		})
	}
	// These existing declarations exercise supported nested and array data,
	// form uploads, and load-deferred data under the same generator checks.
	if err := Check(cfg); err != nil {
		t.Fatalf("supported example wire values: %v", err)
	}
}

func TestCheckAcceptsNullableWithGeneratorProjection(t *testing.T) {
	app := sandboxExample(t)
	web := filepath.Join(app, "web")
	out := filepath.Join(app, "internal", "skgo")
	target := filepath.Join(web, "src", "routes", "optional", "optional.remote.go")
	original, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	const anchor = "type Input struct {"
	if !bytes.Contains(original, []byte(anchor)) {
		t.Fatal("missing optional input fixture")
	}
	valid := strings.Replace(string(original), anchor, anchor+"\n\tNickname polytype.Nullable[string] `json:\"nickname\"`", 1)
	if err := os.WriteFile(target, []byte(valid), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Web: web, Out: out}
	if err := Run(cfg); err != nil {
		t.Fatalf("generator rejected supported nullable value: %v", err)
	}
	if err := Check(cfg); err != nil {
		t.Fatalf("checker rejected generated nullable value: %v", err)
	}
}

func TestNamedDeferredPayloadSerializedNames(t *testing.T) {
	app := sandboxExample(t)
	web := filepath.Join(app, "web")
	out := filepath.Join(app, "internal", "skgo")
	target := filepath.Join(web, "src", "routes", "stream", "page.server.go")
	original, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	const anchor = "// Panel is one section of the page."
	const field = "\n\tPayload skgo.Deferred[Payload] `json:\"payload\"`"
	const declaration = "type Payload struct {\n\tFirst string `json:\"same\"`\n\tSecond string `json:\"same\"`\n}\n\n"
	plant := func(payload, representation string) {
		t.Helper()
		contents := strings.Replace(string(original), anchor, payload+anchor, 1)
		contents = strings.Replace(contents, "type PageData struct {", "type PageData struct {"+strings.Replace(field, "skgo.Deferred[Payload]", representation, 1), 1)
		if strings.Contains(payload, "polytype.Nullable") {
			contents = strings.Replace(contents, "\"github.com/tylergannon/skgo\"", "\"github.com/tylergannon/skgo\"\n\t\"github.com/tylergannon/polytype\"", 1)
		}
		if contents == string(original) || !strings.Contains(contents, payload) || !strings.Contains(contents, representation) {
			t.Fatal("stream fixture anchor missing")
		}
		if err := os.WriteFile(target, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := Config{Web: web, Out: out}
	plant(declaration, "skgo.Deferred[Payload]")
	badSource, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	fieldOffset := strings.Index(string(badSource), "Second string")
	if fieldOffset < 0 {
		t.Fatal("planted duplicate field is missing")
	}
	wantFieldLocation := target + ":" + strconv.Itoa(strings.Count(string(badSource[:fieldOffset]), "\n")+1) + ":"
	for _, tc := range []struct {
		name string
		run  func(Config) error
	}{{"generate", Run}, {"check", Check}} {
		err := tc.run(cfg)
		if err == nil || !strings.Contains(err.Error(), wantFieldLocation) || !strings.Contains(err.Error(), "Second") || !strings.Contains(err.Error(), `duplicate serialized name "same"`) || !strings.Contains(err.Error(), "distinct json name") {
			t.Errorf("%s accepted named Deferred payload's duplicate authored fields: %v", tc.name, err)
		}
	}
	valid := strings.Replace(declaration, "Second string `json:\"same\"`", "Second string `json:\"second\"`\n\tMaybe polytype.Nullable[string] `json:\"maybe\"`", 1)
	for _, tc := range []struct{ name, representation string }{
		{"named", "skgo.Deferred[Payload]"},
		{"slice", "skgo.Deferred[[]Payload]"},
		{"array", "skgo.Deferred[[2]Payload]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plant(valid, tc.representation)
			if err := Run(cfg); err != nil {
				t.Fatalf("generate rejected valid %s Deferred payload: %v", tc.name, err)
			}
			if err := Check(cfg); err != nil {
				t.Fatalf("check rejected valid %s Deferred payload: %v", tc.name, err)
			}
		})
	}
}
