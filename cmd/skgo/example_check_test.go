//go:build skgo_qualification

package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
)

// A red `skgo check` on the example has to mean a new problem, so the example
// at HEAD must pass it with the supported toolchain on PATH: the pinned Go,
// Staticcheck built for it, goimports and the example's installed frontend.
func TestCheckPassesOnTheUnmodifiedExample(t *testing.T) {
	example, err := filepath.Abs(filepath.Join("..", "..", "example"))
	if err != nil {
		t.Fatal(err)
	}
	output, runErr := exec.CommandContext(t.Context(), skgoBinary(t), "check", "--root", example, "--json").Output()
	var report struct {
		OK     bool `json:"ok"`
		Checks []struct{ Name, Status, Message string }
		Diags  []struct{ Source, Code, Message string } `json:"diagnostics"`
	}
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatalf("skgo check did not write a JSON report: %v (run error %v)\n%s", err, runErr, output)
	}
	if len(report.Checks) != 9 {
		t.Fatalf("skgo check returned %d checks, want 9\n%s", len(report.Checks), output)
	}
	for _, c := range report.Checks {
		if c.Status != "complete" {
			t.Errorf("check %s is %s: %s", c.Name, c.Status, c.Message)
		}
	}
	for _, d := range report.Diags {
		t.Errorf("%s/%s: %s", d.Source, d.Code, d.Message)
	}
	if runErr != nil || !report.OK {
		t.Fatalf("skgo check on the example: ok=%v, run error %v", report.OK, runErr)
	}
}
