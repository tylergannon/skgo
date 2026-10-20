//go:build skgo_dev_integration

package devserver

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRealSessionClientHandoff runs example/web/tests/client-handoff.spec.ts
// against a real development session serving a temp copy of the example: Kit's
// client must adopt the page Vite rendered with Go's data, and the open page
// must pick up a Go edit once the session publishes the rebuild.
func TestRealSessionClientHandoff(t *testing.T) {
	root := sessionRepository(t)
	guardSharedKitTypes(t, root)
	temp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(temp, "example")
	copySessionExample(t, root, app)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := ln.Addr().String()
	ln.Close()
	log := &sessionLog{}
	sessionCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- Run(sessionCtx, Options{Root: app, Listen: address, StrictPort: true, Output: log}) }()
	t.Cleanup(func() { stopSession(t, ctx, stop, done, log) })
	origin := "http://" + address

	// The public origin holds requests until the first Go host is admitted.
	for {
		request, err := http.NewRequestWithContext(ctx, "GET", origin+"/todos", nil)
		if err != nil {
			t.Fatal(err)
		}
		if res, err := http.DefaultClient.Do(request); err == nil {
			body, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode == 200 && strings.Contains(string(body), "write the adapter") {
				break
			}
		}
		select {
		case err := <-done:
			done <- err
			t.Fatalf("session exited before serving /todos: %v\n%s", err, log.String())
		case <-ctx.Done():
			t.Fatalf("session never served /todos\n%s", log.String())
		case <-time.After(100 * time.Millisecond):
		}
	}
	runClientHandoff(t, filepath.Join(root, "example/web"), origin, "dev", log, "SKGO_DEV_APP="+app)
}

// copySessionExample copies the example's Go and web sources to app, keeping
// its module path. Like a fresh clone it has no build output; skgo points at
// this checkout and the packages are the example's own install.
func copySessionExample(t *testing.T, root, app string) {
	t.Helper()
	source := filepath.Join(root, "example")
	skip := map[string]bool{".skgo": true, "tmp": true}
	for _, dir := range []string{"node_modules", ".svelte-kit", "test-results"} {
		skip[filepath.Join("web", dir)] = true
	}
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if skip[rel] || (filepath.Dir(rel) == filepath.Join("web", "build") && entry.Name() != ".gitkeep") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(app, rel)
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			// The generated _links are relative and resolve inside the copy.
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case entry.IsDir():
			return os.MkdirAll(target, 0755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, body, info.Mode().Perm())
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "go.mod"), []byte(sessionGoMod(t, root, "github.com/tylergannon/skgo/example")), 0644); err != nil {
		t.Fatal(err)
	}
	linkSessionModules(t, root, filepath.Join(app, "web/node_modules"))
}

// runClientHandoff runs the browser check through example/web's pinned
// Playwright and requires exactly one test to have run and passed. A failure's
// trace and screenshot stay in example/web/test-results/<mode>.
func runClientHandoff(t *testing.T, web, baseURL, mode string, log *sessionLog, env ...string) {
	t.Helper()
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
		t.Fatalf("client handoff check: %v, report: %v, stats %+v, want exactly one passed test; output in %s\n%s\nsession log:\n%s", err, readErr, result.Stats, filepath.Join(web, "test-results", mode), output, log.String())
	}
	t.Logf("%s", output)
}
