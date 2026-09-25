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

// checkLane is one copy of the example that the in-process Check tests below
// plant their wire errors in, one test at a time, each putting the file back
// before it lets go. Check type-checks every route package, and in a fresh
// copy every one of them compiles cold; in the same copy, only the one file a
// test changed does.
var checkLane struct {
	sync.Mutex
	app string
}

var checkLaneSandbox = sync.OnceValues(func() (string, error) { return sharedSandbox("check") })

// lockCheckLane waits for the lane and returns its app. The caller restores
// what it planted before calling unlock.
func lockCheckLane(t *testing.T) (app string, unlock func()) {
	t.Helper()
	app, err := checkLaneSandbox()
	if err != nil {
		t.Fatal(err)
	}
	checkLane.Lock()
	return app, checkLane.Unlock
}

// plantInLane writes planted over path for the rest of the test's hold on the
// lane, and puts original back when the test ends, before the lane is
// released.
func plantInLane(t *testing.T, path string, original, planted []byte, unlock func()) {
	t.Helper()
	t.Cleanup(func() {
		if err := os.WriteFile(path, original, 0o644); err != nil {
			t.Errorf("restoring %s in the check lane: %v", path, err)
		}
		unlock()
	})
	if err := os.WriteFile(path, planted, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadOnlyCheckFindsCurrentWireFieldBeforeStaleLink(t *testing.T) {
	t.Parallel()
	// The planted half holds the check lane, and gives it back before waiting
	// on the valid counterpart.
	t.Run("planted", func(t *testing.T) {
		app, unlock := lockCheckLane(t)
		web := filepath.Join(app, "web")
		out := filepath.Join(app, "internal", "skgo")
		target := filepath.Join(web, "src", "routes", "todos", "todos.remote.go")
		original, err := os.ReadFile(target)
		if err != nil {
			unlock()
			t.Fatal(err)
		}
		const declaration = "type Rename struct {"
		if !bytes.Contains(original, []byte(declaration)) {
			unlock()
			t.Fatal("example no longer declares Rename; update the test")
		}
		broken := []byte(strings.Replace(string(original), declaration, declaration+"\n\tCallback func() `json:\"callback\"`", 1))
		plantInLane(t, target, original, broken, unlock)
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
	})
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
	cmd := exec.Command(bin, "check", "--root", app, "--json")
	cmd.Dir = app
	output, err := cmd.CombinedOutput()
	return cliRun{output, err}
}

// plantedWireError is one unsupported wire value planted in the example for
// the CLI to report. field names the case.
type plantedWireError struct {
	file, anchor, insertion, field, reason, repair string
}

var cliWireErrors = []plantedWireError{
	{"todos/todos.remote.go", "type Rename struct {", "\n\tDetail []map[string]string `json:\"detail\"`", "Detail", "map", "named struct"},
	{"todos/todos.remote.go", "ID string `json:\"id\"`", "\n\tAlias string `json:\"id\"`", "Alias", "duplicate serialized name", "distinct json name"},
	{"todos/todos.remote.go", "type Rename struct {", "\n\tLater skgo.Deferred[string] `json:\"later\"`", "Later", "Deferred", "server load"},
	{"contact/contact.remote.go", "type Receipt struct {", "\n\tUpload skgo.File `json:\"upload\"`", "sendMessage", "skgo.File", "download URL"},
}

// plantedRun is the CLI over one planted wire error, and the line the planted
// field landed on.
type plantedRun struct {
	cliRun
	setupErr error
	wantLine int
}

// cliLane is `skgo check` over copies of the example: untouched, then with
// each of cliWireErrors planted in turn and taken out again. A copy is run in
// sequence rather than one copy per state, because the CLI builds, vets and
// statically checks the whole module: in a fresh directory every package is
// cold, while in the same directory only the one file each case changes is.
type cliLaneRuns struct {
	setupErr error
	pristine func() cliRun
	planted  []func() plantedRun
}

var cliLane = sync.OnceValue(func() *cliLaneRuns {
	lane := &cliLaneRuns{}
	bin, err := skgoBinary()
	if err != nil {
		lane.setupErr = err
		return lane
	}
	// Two copies, so the chain is not all five runs long: the untouched run
	// and the first planted case in one, the other three in the other.
	first, err := sharedSandbox("cli-1")
	if err == nil {
		var second string
		second, err = sharedSandbox("cli-2")
		if err == nil {
			pristine := make(chan cliRun, 1)
			planted := make([]chan plantedRun, len(cliWireErrors))
			for i := range planted {
				planted[i] = make(chan plantedRun, 1)
			}
			go func() {
				pristine <- runCheckCLI(bin, first)
				planted[0] <- plantAndCheck(bin, first, cliWireErrors[0])
			}()
			go func() {
				for i := 1; i < len(cliWireErrors); i++ {
					planted[i] <- plantAndCheck(bin, second, cliWireErrors[i])
				}
			}()
			lane.pristine = sync.OnceValue(func() cliRun { return <-pristine })
			for _, c := range planted {
				lane.planted = append(lane.planted, sync.OnceValue(func() plantedRun { return <-c }))
			}
		}
	}
	lane.setupErr = err
	return lane
})

// plantAndCheck plants tc in app, runs the CLI, and puts the file back.
func plantAndCheck(bin, app string, tc plantedWireError) plantedRun {
	path := filepath.Join(app, "web", "src", "routes", filepath.FromSlash(tc.file))
	original, err := os.ReadFile(path)
	if err != nil {
		return plantedRun{setupErr: err}
	}
	planted := strings.Replace(string(original), tc.anchor, tc.anchor+tc.insertion, 1)
	if planted == string(original) {
		return plantedRun{setupErr: fmt.Errorf("%s: fixture anchor %q missing", tc.file, tc.anchor)}
	}
	wantLine := strings.Count(planted[:strings.Index(planted, tc.anchor)+len(tc.anchor)], "\n") + 2
	if err := os.WriteFile(path, []byte(planted), 0o644); err != nil {
		return plantedRun{setupErr: err}
	}
	run := runCheckCLI(bin, app)
	if err := os.WriteFile(path, original, 0o644); err != nil {
		return plantedRun{setupErr: fmt.Errorf("restoring %s: %w", tc.file, err)}
	}
	return plantedRun{cliRun: run, wantLine: wantLine}
}

// pristineCLI is `skgo check` over the untouched example, the lane's first
// run. TestRealCheckReportsWireAdviceAtAuthoredLocations asserts the whole
// report; pristineCheck reads the generator's own check out of it.
func pristineCLI() (cliRun, error) {
	lane := cliLane()
	if lane.setupErr != nil {
		return cliRun{}, lane.setupErr
	}
	return lane.pristine(), nil
}

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
	lane := cliLane()
	if lane.setupErr != nil {
		t.Fatal(lane.setupErr)
	}
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
	for i, tc := range cliWireErrors {
		t.Run(tc.field, func(t *testing.T) {
			run := lane.planted[i]()
			if run.setupErr != nil {
				t.Fatal(run.setupErr)
			}
			got := parse(t, run.cliRun, true)
			found := false
			for _, d := range got.Diagnostics {
				if d.Code == "SKGO007" && strings.Contains(d.Message, tc.reason) && strings.Contains(d.Message, tc.repair) && d.Location != nil && d.Location.File == filepath.ToSlash(filepath.Join("web", "src", "routes", tc.file)) && d.Location.Line == run.wantLine {
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
			app, unlock := lockCheckLane(t)
			web := filepath.Join(app, "web")
			file := filepath.Join(web, "src", "routes", filepath.FromSlash(tc.file))
			original, err := os.ReadFile(file)
			if err != nil {
				unlock()
				t.Fatal(err)
			}
			if !bytes.Contains(original, []byte(tc.anchor)) {
				unlock()
				t.Fatalf("missing fixture anchor %q", tc.anchor)
			}
			planted := strings.Replace(string(original), tc.anchor, tc.anchor+tc.insertion, 1)
			plantInLane(t, file, original, []byte(planted), unlock)
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
