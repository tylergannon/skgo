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

func TestCheckReportsRefreshAndRouteAdviceAtAuthoredLocations(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	const route = "web/src/routes/[customerID]/[[tab]]/[...tail]"
	const link = "internal/skgo/links/onzggl3sn52xizltf5nwg5ltorxw2zlsjfcf2l23ln2gcys5luxvwlrofz2gc2lmlu"
	for _, dir := range []string{"web/src/lib", route, link} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name, contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/refreshcheck\n\ngo 1.27.1\n\nrequire github.com/tylergannon/skgo v0.0.0\n\nreplace github.com/tylergannon/skgo => "+repo+"\n")
	write("web/package.json", "{\"name\":\"refreshcheck\",\"private\":true,\"type\":\"module\"}\n")
	write("web/src/routes/go.mod", "module example.com/refreshcheck/routes\n\ngo 1.27.1\n\nrequire github.com/tylergannon/skgo v0.0.0\n\nreplace github.com/tylergannon/skgo => "+repo+"\n")
	fixture := filepath.Join(repo, "internal", "advice", "testdata", "src")
	remote, err := os.ReadFile(filepath.Join(fixture, "r", "r.remote.go"))
	if err != nil {
		t.Fatal(err)
	}
	write("web/src/lib/target.remote.go", strings.Replace(string(remote), "package r", "package app", 1))
	page, err := os.ReadFile(filepath.Join(fixture, "a", "links", filepath.Base(link), "page.server.go"))
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(route, "page.server.go"), string(page))
	write(filepath.Join(link, "page.server.go"), string(page))
	helper, err := os.ReadFile(filepath.Join(fixture, "a", "links", filepath.Base(link), "helper.go"))
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(route, "helper.go"), string(helper))
	write(filepath.Join(link, "helper.go"), string(helper))
	write("internal/skgo/links.json", "{\"links\":[{\"link\":\""+link+"\",\"target\":\""+route+"\"}]}\n")

	command := func(dir, name string, args ...string) ([]byte, error) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		return cmd.CombinedOutput()
	}
	if output, err := command(root, "go", "mod", "tidy"); err != nil {
		t.Fatalf("prepare CLI fixture: %v\n%s", err, output)
	}
	bin := filepath.Join(root, "skgo")
	if output, err := command(repo, "go", "build", "-o", bin, "./cmd/skgo"); err != nil {
		t.Fatalf("build real CLI: %v\n%s", err, output)
	}
	output, runErr := command(root, bin, "check", "--root", root, "--json")
	var report check.Report
	if err := json.Unmarshal(output, &report); err != nil {
		t.Fatalf("CLI returned incomplete JSON: %v; run error: %v\n%s", err, runErr, output)
	}
	var exit *exec.ExitError
	if !errors.As(runErr, &exit) || exit.ExitCode() != 1 || report.OK {
		t.Fatalf("CLI should reject the planted misuse: run=%v report.OK=%v", runErr, report.OK)
	}
	complete := false
	for _, c := range report.Checks {
		if c.Name == "skgo-advice" {
			complete = c.Status == "complete"
		}
	}
	if !complete {
		t.Fatalf("skgo-advice did not complete: %+v", report.Checks)
	}
	counts := map[string]int{}
	for _, d := range report.Diagnostics {
		if d.Source != "skgo-advice" || (d.Code != "SKGO005" && d.Code != "SKGO008") {
			continue
		}
		counts[d.Code]++
		wantFile := "web/src/lib/target.remote.go"
		if d.Code == "SKGO008" {
			wantFile = route + "/helper.go"
		}
		if d.Location == nil || d.Location.File != wantFile || d.Location.Line == 0 || d.Location.EndColumn <= d.Location.Column {
			t.Errorf("advice has no useful authored location: %+v", d)
		}
		if d.Documentation != "skgo advice "+d.Code {
			t.Errorf("diagnostic does not lead to its rule advice: %+v", d)
		}
		if d.Code == "SKGO005" && (!strings.Contains(d.Message, "registered") || !strings.Contains(d.Message, "rejects")) {
			t.Errorf("refresh advice lacks consequence or repair: %+v", d)
		}
		if d.Code == "SKGO008" && (!strings.Contains(d.Message, "empty string") || !strings.Contains(d.Message, "declared")) {
			t.Errorf("route advice lacks consequence or repair: %+v", d)
		}
	}
	if counts["SKGO005"] != 6 || counts["SKGO008"] != 1 {
		t.Fatalf("CLI advice = %v; want six target mismatches and one absent route parameter", counts)
	}
}
