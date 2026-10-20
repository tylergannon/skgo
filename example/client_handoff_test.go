//go:build skgo_qualification

package example_test

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBuiltClientHandoff runs example/web/tests/client-handoff.spec.ts against
// the built example: Kit's client must adopt the page Go rendered with goja.
// Kit bakes the origin into the build, so the handler is served on it.
func TestBuiltClientHandoff(t *testing.T) {
	origin := cmp.Or(os.Getenv("ORIGIN"), prodOrigin)
	if origin != prodOrigin {
		t.Fatalf("ORIGIN is %s, but the example tests' handler is configured for %s", origin, prodOrigin)
	}
	address, err := url.Parse(origin)
	if err != nil {
		t.Fatal(err)
	}
	handler := newProdHandler(t)
	listener, err := net.Listen("tcp", address.Host)
	if err != nil {
		t.Fatalf("the build must be served on %s, the origin it was built for: %v", origin, err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("serving the build: %v", err)
		}
	}()
	t.Cleanup(func() { server.Close() })
	runClientHandoff(t, origin, "production")
}

// runClientHandoff runs the browser check through example/web's pinned
// Playwright and requires exactly one test to have run and passed. A failure's
// trace and screenshot stay in example/web/test-results/<mode>.
func runClientHandoff(t *testing.T, baseURL, mode string, env ...string) {
	t.Helper()
	web, err := filepath.Abs("web")
	if err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(t.TempDir(), "report.json")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "vp", "exec", "playwright", "test", "--output", "test-results/"+mode, "--reporter", "list,json")
	command.Dir = web
	command.Env = append(append(os.Environ(), "BASE_URL="+baseURL, "PLAYWRIGHT_JSON_OUTPUT_FILE="+report), env...)
	output, err := command.CombinedOutput()
	if strings.Contains(string(output), "Executable doesn't exist") {
		t.Fatalf("Playwright's Chromium is not installed; run `vp exec playwright install chromium` in example/web\n%s", output)
	}
	var result struct {
		Stats struct{ Expected, Skipped, Unexpected, Flaky int } `json:"stats"`
	}
	raw, readErr := os.ReadFile(report)
	if readErr == nil {
		readErr = json.Unmarshal(raw, &result)
	}
	if err != nil || readErr != nil || result.Stats.Expected != 1 || result.Stats.Skipped+result.Stats.Unexpected+result.Stats.Flaky != 0 {
		t.Fatalf("client handoff check: %v, report: %v, stats %+v, want exactly one passed test; output in %s\n%s", err, readErr, result.Stats, filepath.Join(web, "test-results", mode), output)
	}
	t.Logf("%s", output)
}
