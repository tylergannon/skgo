package gen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestReadOnlyCheckFindsCurrentWireFieldBeforeStaleLink(t *testing.T) {
	t.Parallel()
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
	// The valid counterpart is the untouched example, which pristineCheck
	// checks once for every test that needs it.
	if err := pristineCheck(); err != nil {
		t.Fatalf("valid counterpart did not check cleanly: %v", err)
	}
}

// cliReport is the part of `skgo check --json` these tests read.
type cliReport struct {
	OK          bool
	Checks      []struct{ Name, Status, Message string }
	Diagnostics []struct {
		Code, Message string
		Location      *struct {
			File string
			Line int
		}
	}
}

// cliRun is one `skgo check --root <app> --json` and what it printed.
type cliRun struct {
	output []byte
	err    error
}

func runCheckCLI(bin, app string) cliRun {
	defer tlog("cli " + app)()
	cmd := exec.Command(bin, "check", "--root", app, "--json")
	cmd.Dir = app
	output, err := cmd.CombinedOutput()
	return cliRun{output, err}
}

// pristineCLI is `skgo check` over an untouched copy of the example, run once.
// TestRealCheckReportsWireAdviceAtAuthoredLocations asserts the whole report;
// pristineCheck reads the generator's own check out of it.
var pristineCLI = sync.OnceValues(func() (cliRun, error) {
	bin, err := skgoBinary()
	if err != nil {
		return cliRun{}, err
	}
	app, err := sharedSandbox("pristine")
	if err != nil {
		return cliRun{}, err
	}
	return runCheckCLI(bin, app), nil
})

// pristineCheck is the generator's Check over the untouched example: the
// valid counterpart every test that plants a wire error needs to check
// cleanly. It is the same source every time, so it is checked once, by the
// CLI run above, which calls Check with the same Config these tests would
// and reports its error as the "skgo" check's message.
func pristineCheck() error {
	run, err := pristineCLI()
	if err != nil {
		return err
	}
	var got cliReport
	if err := json.Unmarshal(run.output, &got); err != nil {
		return fmt.Errorf("skgo check returned incomplete JSON: %v; run=%v\n%s", err, run.err, run.output)
	}
	for _, c := range got.Checks {
		if c.Name == "skgo" {
			if c.Status != "complete" {
				return fmt.Errorf("skgo check: generator check %s: %s", c.Status, c.Message)
			}
			return nil
		}
	}
	return fmt.Errorf("skgo check reported no generator check: %s", run.output)
}

func TestRealCheckReportsWireAdviceAtAuthoredLocations(t *testing.T) {
	t.Parallel()
	bin := requireSkgoBinary(t)
	go pristineCLI() // started now; the supported case below waits for it
	parse := func(t *testing.T, run cliRun, expectWireFailure bool) cliReport {
		t.Helper()
		output, runErr := run.output, run.err
		var got cliReport
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
	// Each planted state is its own sandbox, and every CLI run — one per
	// distinct source state, since the generator stops at the first wire error
	// — starts before any is waited for, so they run at once rather than each
	// queueing for one of the test runner's parallel slots.
	type planted struct {
		file, reason, repair string
		wantLine             int
		run                  func() cliRun
	}
	var cases []planted
	var names []string
	for _, tc := range []struct {
		file, anchor, insertion, field, reason, repair string
	}{
		{"todos/todos.remote.go", "type Rename struct {", "\n\tDetail []map[string]string `json:\"detail\"`", "Detail", "map", "named struct"},
		{"todos/todos.remote.go", "ID string `json:\"id\"`", "\n\tAlias string `json:\"id\"`", "Alias", "duplicate serialized name", "distinct json name"},
		{"todos/todos.remote.go", "type Rename struct {", "\n\tLater skgo.Deferred[string] `json:\"later\"`", "Later", "Deferred", "server load"},
		{"contact/contact.remote.go", "type Receipt struct {", "\n\tUpload skgo.File `json:\"upload\"`", "sendMessage", "skgo.File", "download URL"},
	} {
		app := sandboxExample(t)
		path := filepath.Join(app, "web", "src", "routes", tc.file)
		original, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		plantedSource := strings.Replace(string(original), tc.anchor, tc.anchor+tc.insertion, 1)
		if plantedSource == string(original) {
			t.Fatalf("%s: fixture anchor missing", tc.field)
		}
		wantLine := strings.Count(plantedSource[:strings.Index(plantedSource, tc.anchor)+len(tc.anchor)], "\n") + 2
		if err := os.WriteFile(path, []byte(plantedSource), 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, tc.field)
		cases = append(cases, planted{tc.file, tc.reason, tc.repair, wantLine, start(func() cliRun { return runCheckCLI(bin, app) })})
	}
	for i, tc := range cases {
		t.Run(names[i], func(t *testing.T) {
			got := parse(t, tc.run(), true)
			found := false
			for _, d := range got.Diagnostics {
				if d.Code == "SKGO007" && strings.Contains(d.Message, tc.reason) && strings.Contains(d.Message, tc.repair) && d.Location != nil && d.Location.File == filepath.ToSlash(filepath.Join("web", "src", "routes", tc.file)) && d.Location.Line == tc.wantLine {
					found = true
				}
			}
			if !found {
				t.Fatalf("SKGO007 missing authored location and repair: %+v", got.Diagnostics)
			}
		})
	}
	t.Run("supported", func(t *testing.T) {
		run, err := pristineCLI()
		if err != nil {
			t.Fatal(err)
		}
		valid := parse(t, run, false)
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
	})
}

func TestCheckWireAdviceUsesAuthoredDeclarations(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, file, anchor, insertion, consequence, repair string
	}{
		{"nested unsupported", "todos/todos.remote.go", "type Rename struct {", "\n\tDetail []map[string]string `json:\"detail\"`", "cannot cross the wire", "Use a named struct"},
		{"duplicate name", "todos/todos.remote.go", "ID string `json:\"id\"`", "\n\tAlias string `json:\"id\"`", "duplicate serialized name", "distinct json name"},
		{"deferred remote", "todos/todos.remote.go", "type Rename struct {", "\n\tLater skgo.Deferred[string] `json:\"later\"`", "Deferred", "server load"},
		{"file result", "contact/contact.remote.go", "type Receipt struct {", "\n\tUpload skgo.File `json:\"upload\"`", "returns a skgo.File", "download URL"},
		{"deferred file result", "stream/page.server.go", "type PageData struct {", "\n\tUpload skgo.Deferred[skgo.File] `json:\"upload\"`", "returns a skgo.File", "download URL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			app := sandboxExample(t)
			web := filepath.Join(app, "web")
			file := filepath.Join(web, "src", "routes", filepath.FromSlash(tc.file))
			original, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(original, []byte(tc.anchor)) {
				t.Fatalf("missing fixture anchor %q", tc.anchor)
			}
			planted := strings.Replace(string(original), tc.anchor, tc.anchor+tc.insertion, 1)
			if err := os.WriteFile(file, []byte(planted), 0o644); err != nil {
				t.Fatal(err)
			}
			err = Check(Config{Web: web, Out: filepath.Join(app, "internal", "skgo")})
			if err == nil || !strings.Contains(err.Error(), tc.consequence) || !strings.Contains(err.Error(), tc.repair) || !strings.Contains(err.Error(), filepath.Base(file)+":") {
				t.Fatalf("wanted authored wire advice with consequence and repair, got %v", err)
			}
		})
	}
	// The untouched example's declarations exercise supported nested and array
	// data, form uploads, and load-deferred data under the same generator
	// checks.
	t.Run("supported", func(t *testing.T) {
		t.Parallel()
		if err := pristineCheck(); err != nil {
			t.Fatalf("supported example wire values: %v", err)
		}
	})
}

// The evolved example (evolution_test.go) added a polytype.Nullable field to
// a Form input in the optional route.
func TestCheckAcceptsNullableWithGeneratorProjection(t *testing.T) {
	t.Parallel()
	e := requireEvolved(t) // fails with the generator's refusal if it rejected the nullable value
	if err := e.check(); err != nil {
		t.Fatalf("checker rejected generated nullable value: %v", err)
	}
}

func TestNamedDeferredPayloadSerializedNames(t *testing.T) {
	t.Parallel()
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
	contents := strings.Replace(string(original), anchor, declaration+anchor, 1)
	contents = strings.Replace(contents, "type PageData struct {", "type PageData struct {"+field, 1)
	if contents == string(original) || !strings.Contains(contents, declaration) || !strings.Contains(contents, field) {
		t.Fatal("stream fixture anchor missing")
	}
	if err := os.WriteFile(target, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Web: web, Out: out}
	fieldOffset := strings.Index(contents, "Second string")
	if fieldOffset < 0 {
		t.Fatal("planted duplicate field is missing")
	}
	wantFieldLocation := target + ":" + strconv.Itoa(strings.Count(contents[:fieldOffset], "\n")+1) + ":"
	for _, tc := range []struct {
		name string
		run  func(Config) error
	}{{"generate", Run}, {"check", Check}} {
		err := tc.run(cfg)
		if err == nil || !strings.Contains(err.Error(), wantFieldLocation) || !strings.Contains(err.Error(), "Second") || !strings.Contains(err.Error(), `duplicate serialized name "same"`) || !strings.Contains(err.Error(), "distinct json name") {
			t.Errorf("%s accepted named Deferred payload's duplicate authored fields: %v", tc.name, err)
		}
	}

	// The valid counterpart — distinct names and a nullable member, reached by
	// value, through a slice and through an array — is planted in the evolved
	// example, all three representations in one PageData, so one generation
	// and one check accept or refuse all three.
	e := requireEvolved(t) // fails with the generator's refusal if it rejected any of them
	source, err := os.ReadFile(filepath.Join(e.first.dir, filepath.FromSlash(streamSource)))
	if err != nil {
		t.Fatal(err)
	}
	checkErr := e.check()
	for _, r := range deferredRepresentations {
		t.Run(r.name, func(t *testing.T) {
			if !strings.Contains(string(source), deferredPayload) || !strings.Contains(string(source), r.field) {
				t.Fatalf("the evolved example does not carry the %s Deferred payload", r.name)
			}
			if checkErr != nil {
				t.Fatalf("check rejected valid %s Deferred payload: %v", r.name, checkErr)
			}
		})
	}
}
