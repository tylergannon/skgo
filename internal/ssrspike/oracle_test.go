package ssrspike

import (
	"encoding/json"
	"os/exec"
	"testing"
)

type oracleResult struct {
	Done  bool   `json:"done"`
	Error string `json:"error"`
	Head  string `json:"head"`
	Body  string `json:"body"`
}

// TestMatchesNodeOracle runs the identical bundle through Node and compares the
// HTML byte for byte. Node is the reference implementation of "what SvelteKit
// would have produced"; any difference is a goja difference, because the
// JavaScript is the same file.
//
// A missing Node is a failure, not a skip: an SSR claim that has never been
// checked against the reference is not a claim.
func TestMatchesNodeOracle(t *testing.T) {
	dir := spikeDir(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node is not on PATH, so the oracle cannot run and this comparison has not been made: %v", err)
	}

	cmd := exec.Command(node, "oracle.mjs")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("oracle failed: %v\n%s", err, ee.Stderr)
		}
		t.Fatalf("oracle failed: %v", err)
	}

	var oracle map[string]oracleResult
	if err := json.Unmarshal(out, &oracle); err != nil {
		t.Fatalf("decoding oracle output: %v\n%s", err, out)
	}

	p, f := load(t)
	if len(oracle) == 0 {
		t.Fatal("the oracle produced no scenarios")
	}

	for name := range f.Requests {
		want, ok := oracle[name]
		if !ok {
			t.Errorf("oracle has no result for scenario %q", name)
			continue
		}
		if !want.Done || want.Error != "" {
			t.Errorf("oracle scenario %q did not render cleanly: done=%v error=%s", name, want.Done, want.Error)
			continue
		}

		e, err := New(p.p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := e.Render(f.Requests[name], answerer(t, f))
		if err != nil {
			t.Errorf("goja render %q: %v", name, err)
			continue
		}

		switch {
		case !got.Done:
			t.Errorf("%s: goja render never settled", name)
		case got.Error != "":
			t.Errorf("%s: goja render threw: %s", name, got.Error)
		case got.Body != want.Body:
			t.Errorf("%s: body differs between goja and node\ngoja: %s\nnode: %s", name, got.Body, want.Body)
		case got.Head != want.Head:
			t.Errorf("%s: head differs between goja and node\ngoja: %q\nnode: %q", name, got.Head, want.Head)
		default:
			t.Logf("%s: identical (%d bytes of body)", name, len(got.Body))
		}
	}
}
