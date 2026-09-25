package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/check"
)

func TestCheckReportsQueryAdviceAtAuthoredLocations(t *testing.T) {
	t.Parallel()
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, dir := range []string{"src/routes", "web/src"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	fixture, err := os.ReadFile(filepath.Join(repo, "internal", "advice", "testdata", "src", "q", "q.go"))
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/querycheck\n\ngo 1.27.1\n\nrequire github.com/tylergannon/skgo v0.0.0\n\nreplace github.com/tylergannon/skgo => "+repo+"\n")
	write("web/package.json", "{\"name\":\"querycheck\",\"private\":true,\"type\":\"module\"}\n")
	write("src/routes/query.remote.go", strings.Replace(string(fixture), "package q", "package routes", 1))

	command := func(dir, name string, args ...string) ([]byte, error) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		return cmd.CombinedOutput()
	}
	if output, err := command(root, "go", "mod", "tidy"); err != nil {
		t.Fatalf("prepare CLI fixture: %v\n%s", err, output)
	}
	bin := skgoBin
	output, runErr := command(root, bin, "check", "--root", root, "--json")
	var report check.Report
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatalf("CLI returned incomplete JSON: %v; run error: %v\n%s", err, runErr, output)
	}
	var exit *exec.ExitError
	if !errors.As(runErr, &exit) || exit.ExitCode() != 1 || report.OK {
		t.Fatalf("CLI should report planted misuse as a failed check: run=%v report.OK=%v", runErr, report.OK)
	}
	adviceComplete := false
	for _, c := range report.Checks {
		if c.Name == "skgo-advice" {
			adviceComplete = c.Status == "complete"
		}
	}
	if !adviceComplete {
		t.Fatalf("skgo-advice did not complete: %+v", report.Checks)
	}
	counts := map[string]int{}
	for _, d := range report.Diagnostics {
		if d.Source != "skgo-advice" {
			continue
		}
		counts[d.Code]++
		if d.Location == nil || d.Location.File != "src/routes/query.remote.go" || d.Location.Line == 0 || d.Location.EndColumn <= d.Location.Column {
			t.Errorf("advice has no useful authored location: %+v", d)
		}
		if d.Code == "SKGO001" && (!strings.Contains(d.Message, "Kit refuses") || !strings.Contains(d.Message, "command or form")) {
			t.Errorf("cookie advice lacks consequence or repair: %+v", d)
		}
		if d.Code == "SKGO002" && (!strings.Contains(d.Message, "cache key") || !strings.Contains(d.Message, "typed query argument")) {
			t.Errorf("page advice lacks consequence or repair: %+v", d)
		}
	}
	if counts["SKGO001"] != 4 || counts["SKGO002"] != 6 || len(counts) != 2 {
		t.Fatalf("CLI advice = %v; want only four SKGO001 and six SKGO002 findings", counts)
	}
}
