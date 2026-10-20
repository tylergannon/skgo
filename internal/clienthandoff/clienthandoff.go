// Package clienthandoff runs example/web/tests/client-handoff.test.ts, the one
// browser check: Kit's client adopting the HTML and data Go rendered. The
// caller serves the application; Run starts Lightpanda, a headless browser
// with no layout or rendering, and runs the check with Vitest against both.
package clienthandoff

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Install is how to put the pinned Lightpanda on PATH.
const Install = "brew install lightpanda-io/browser/lightpanda, or put https://github.com/lightpanda-io/browser/releases/download/1.0.1/lightpanda-<arch>-<os> on PATH as lightpanda"

// Run runs the check in web, example/web or a copy of it, against origin and
// requires exactly one test to have run and passed. mode names the run; a
// failure leaves the page's HTML in web/test-results/<mode>. env adds to the
// check's environment. It returns Vitest's output.
func Run(t testing.TB, web, origin, mode string, env ...string) (string, error) {
	t.Helper()
	cdp := serveLightpanda(t)
	report := filepath.Join(t.TempDir(), "report.json")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "vp", "test", "run", "--config", "tests/vitest.config.ts",
		"--reporter=default", "--reporter=json", "--outputFile.json="+report)
	command.Dir = web
	command.Env = append(append(os.Environ(),
		"HANDOFF_ORIGIN="+origin, "HANDOFF_MODE="+mode, "LIGHTPANDA_CDP="+cdp), env...)
	output, err := command.CombinedOutput()
	var result struct {
		Total   int `json:"numTotalTests"`
		Passed  int `json:"numPassedTests"`
		Failed  int `json:"numFailedTests"`
		Pending int `json:"numPendingTests"`
		Todo    int `json:"numTodoTests"`
	}
	raw, readErr := os.ReadFile(report)
	if readErr == nil {
		readErr = json.Unmarshal(raw, &result)
	}
	if err != nil || readErr != nil || result.Total != 1 || result.Passed != 1 || result.Failed+result.Pending+result.Todo != 0 {
		return string(output), fmt.Errorf("client handoff check: %v, report: %v, results %+v, want exactly one passed test; page in %s",
			err, readErr, result, filepath.Join(web, "test-results", mode))
	}
	return string(output), nil
}

// serveLightpanda starts Lightpanda's CDP server on a free port for the rest
// of the test and returns its endpoint.
func serveLightpanda(t testing.TB) string {
	t.Helper()
	binary, err := exec.LookPath("lightpanda")
	if err != nil {
		t.Fatalf("the client handoff check drives Lightpanda: %s", Install)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	ln.Close()
	var log bytes.Buffer
	command := exec.Command(binary, "serve", "--host", "127.0.0.1", "--port", port, "--log-level", "warn")
	command.Env = append(os.Environ(), "LIGHTPANDA_DISABLE_TELEMETRY=true")
	command.Stdout, command.Stderr = &log, &log
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	t.Cleanup(func() {
		command.Process.Kill()
		<-exited
		if t.Failed() && log.Len() > 0 {
			t.Logf("lightpanda:\n%s", log.String())
		}
	})
	version := "http://127.0.0.1:" + port + "/json/version"
	deadline := time.Now().Add(10 * time.Second)
	for {
		if res, err := http.Get(version); err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return "ws://127.0.0.1:" + port + "/"
			}
		}
		select {
		case err := <-exited:
			exited <- err
			t.Fatalf("lightpanda exited before serving CDP: %v\n%s", err, log.String())
		case <-time.After(50 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			command.Process.Kill()
			err := <-exited
			exited <- err
			t.Fatalf("lightpanda never served %s: %v\n%s", version, err, log.String())
		}
	}
}
