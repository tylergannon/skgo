package gen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Check mutations share a small generated module. Each assertion restores its
// authored source before releasing the lane.
var checkLane struct {
	sync.Mutex
}

var checkLaneSandbox = sync.OnceValues(func() (string, error) { return cloneCheckModule("check") })

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

func testReadOnlyCheckFindsCurrentWireFieldBeforeStaleLink(t *testing.T) {
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
			t.Fatal("fixture no longer declares Rename; update the test")
		}
		broken := []byte(strings.Replace(string(original), declaration, declaration+"\n\tCallback func() `json:\"callback\"`", 1))
		plantInLane(t, target, original, broken, unlock)
		links, err := filepath.Glob(filepath.Join(out, "links", "*", "todos.remote.go"))
		if err != nil || len(links) != 1 {
			t.Fatalf("expected one generated todos route copy, got %v: %v", links, err)
		}
		link := links[0]
		beforeLink, err := os.ReadFile(link)
		if err != nil {
			t.Fatal(err)
		}
		err = Check(fixtureConfig(Config{Web: web, Out: out}))
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
	// The valid counterpart is the pristine fixture, which pristineCheck
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

// plantedWireError is one unsupported wire value planted in the fixture for
// the CLI to report. field names the case.
type plantedWireError struct {
	file, anchor, insertion, field, reason, repair string
}

var cliWireErrors = []plantedWireError{
	{"todos/todos.remote.go", "type Rename struct {", "\n\tDetail []map[string]string `json:\"detail\"`", "Detail", "map", "named struct"},
}

// plantedRun is the CLI over one planted wire error, and the line the planted
// field landed on.
type plantedRun struct {
	cliRun
	setupErr error
	wantLine int
}

// One pristine and one failing small module exercise the CLI's complete JSON
// report, exit status and authored location. Declaration tests cover the wire
// rules without rebuilding a module for every spelling.
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
	// Independent pristine and failing CLI runs keep CLI-format proof to one
	// representative wire error; direct declarations distinguish the rules.
	first, err := cloneCheckModule("cli-1")
	if err == nil {
		var second string
		second, err = cloneCheckModule("cli-2")
		if err == nil {
			pristine := make(chan cliRun, 1)
			planted := make([]chan plantedRun, len(cliWireErrors))
			for i := range planted {
				planted[i] = make(chan plantedRun, 1)
			}
			go func() {
				pristine <- runCheckCLI(bin, first)
			}()
			go func() {
				for i := 0; i < len(cliWireErrors); i++ {
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

// pristineCLI is `skgo check` over the pristine fixture, the lane's first
// run. TestRealCheckReportsWireAdviceAtAuthoredLocations asserts the whole
// report; pristineCheck reads the generator's own check out of it.
func pristineCLI() (cliRun, error) {
	lane := cliLane()
	if lane.setupErr != nil {
		return cliRun{}, lane.setupErr
	}
	return lane.pristine(), nil
}

// pristineCheck is the generator's Check over the pristine fixture: the
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

func testRealCheckReportsWireAdviceAtAuthoredLocations(t *testing.T) {
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
			t.Fatalf("supported declaration did not complete generator check: %+v", valid.Checks)
		}
	})
}

func TestCheckWireAdviceUsesAuthoredDeclarations(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, field, consequence, repair string
		load                             bool
	}{
		{"nested unsupported", "Detail []map[string]string", "cannot cross the wire", "Use a named struct", false},
		{"duplicate name", "A string `json:\"same\"`; Alias string `json:\"same\"`", "duplicate serialized name", "distinct json name", false},
		{"deferred remote", "Later skgo.Deferred[string]", "Deferred", "server load", false},
		{"file result", "Upload skgo.File", "returns a skgo.File", "download URL", false},
		{"deferred file result", "Upload skgo.Deferred[skgo.File]", "returns a skgo.File", "download URL", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := wireDeclaration(t, "package fixture\nimport \"github.com/tylergannon/skgo\"\nvar _ skgo.File\ntype Output struct{\n"+tc.field+"\n}\n")
			typ := p.Types.Scope().Lookup("Output").Type()
			gp := &goPackage{pkg: p}
			pos := token.Position{Filename: "wire.go", Line: 4}
			a := &app{}
			if tc.load {
				a.loads = []*loadFn{{name: "load", out: typ, goPkg: gp, pos: pos}}
			} else {
				a.remotes = []*remoteFn{{name: "query", out: typ, goPkg: gp, pos: pos}}
			}
			err := a.checkFileUsage()
			if err == nil {
				err = a.checkWireFields()
			}
			if err == nil || !strings.Contains(err.Error(), tc.consequence) || !strings.Contains(err.Error(), tc.repair) || !strings.Contains(err.Error(), "wire.go:") {
				t.Fatalf("missing authored consequence/repair: %v", err)
			}
		})
	}
}

// The evolved example (evolution_test.go) added a polytype.Nullable field to
// a Form input in the optional route.
func testCheckAcceptsNullableWithGeneratorProjection(t *testing.T) {
	e := requireEvolved(t) // fails with the generator's refusal if it rejected the nullable value
	if err := e.check(); err != nil {
		t.Fatalf("checker rejected generated nullable value: %v", err)
	}
}

func testNamedDeferredPayloadSerializedNames(t *testing.T) {
	p := wireDeclaration(t, `package fixture
import "github.com/tylergannon/skgo"
type Payload struct {
 First string `+"`json:\"same\"`"+`
 Second string `+"`json:\"same\"`"+`
}
type Output struct { Value skgo.Deferred[Payload] }
`)
	err := checkTypeSerializedNames(p.Types.Scope().Lookup("Output").Type(), p.Fset, nil)
	if err == nil || !strings.Contains(err.Error(), "wire.go:5:") || !strings.Contains(err.Error(), `duplicate serialized name "same"`) {
		t.Fatalf("lost named Deferred field location: %v", err)
	}
	// Real projection of valid named/slice/array nullable Deferred payloads
	// remains in the shared evolved application and its generated build/check.
	e := requireEvolved(t)
	source, err := os.ReadFile(filepath.Join(e.first.dir, filepath.FromSlash(streamSource)))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range deferredRepresentations {
		if !strings.Contains(string(source), deferredPayload) || !strings.Contains(string(source), r.field) {
			t.Fatalf("missing %s Deferred counterpart", r.name)
		}
	}
	if err := e.check(); err != nil {
		t.Fatal(err)
	}
}
