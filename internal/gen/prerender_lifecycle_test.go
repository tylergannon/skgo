package gen

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testRealKitBuildSharesPrerenderLocalsAndWrapsTheRenderedResponse(t *testing.T) {
	fixture := requireProductionFixture(t)
	for _, slug := range []string{"alpha", "beta"} {
		body, err := os.ReadFile(filepath.Join(fixture.app, "ui", "build", "prerendered", "lifecycle", slug+".html"))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"<h1>Lifecycle</h1>", "/lifecycle/" + slug + ":layout-11", "/lifecycle/" + slug + ":page-12", "/lifecycle/" + slug + ":remote-13 cookies:scoped%20raw deleted", "Go fetch: own locals 10", "<p>Kit fetch: Go fetch: own locals 10</p>", "address unavailable", "deferred:/lifecycle/" + slug, "<!-- Go after hook: /lifecycle/" + slug + " cookies:scoped%20raw deleted cookie-forwarding:root -->"} {
			if !strings.Contains(string(body), want) {
				t.Errorf("%s lacks %q: %s", slug, want, body)
			}
		}
	}
}

func testCompiledProductionPrerenderedEndpointsAndPages(t *testing.T) {
	requireProductionFixture(t).run(t, "TestProductionPrerenderedEndpointsAndPages")
}

func testRealProductionStartsWithPrerenderedEndpoints(t *testing.T) {
	fixture := requireProductionFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(fixture.app, "server"))
	command.Dir = fixture.app
	command.Env = append(os.Environ(), "LISTEN_ADDRESS=127.0.0.1:0")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	stop := func() {
		if !stopped {
			stopped = true
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}
	defer stop()
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			ready <- scanner.Text()
		} else {
			ready <- ""
		}
	}()
	var origin string
	select {
	case origin = <-ready:
	case <-ctx.Done():
		stop()
		t.Fatalf("production startup timed out: %s", stderr.String())
	}
	if !strings.HasPrefix(origin, "http://127.0.0.1:") {
		stop()
		t.Fatalf("production failed before listening: %q %s", origin, stderr.String())
	}
	client := &http.Client{Timeout: 2 * time.Second}
	for _, fixture := range []struct{ path, body string }{
		{"/lifecycle-api", "Go fetch: own locals 10"},
		{"/lifecycle/alpha", "<!-- Go after hook: /lifecycle/alpha cookies:scoped%20raw deleted cookie-forwarding:root -->"},
		{"/ordinary", "<h1>Ordinary SSR route</h1>"},
	} {
		response, err := client.Get(origin + fixture.path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != 200 || !strings.Contains(string(body), fixture.body) {
			t.Fatalf("production %s: %d %s %v", fixture.path, response.StatusCode, body, err)
		}
	}
}

func TestPrerenderHookRefusesBothAuthoredServerHookPaths(t *testing.T) {
	t.Parallel()
	for _, extension := range []string{"ts", "js"} {
		t.Run(extension, func(t *testing.T) {
			const authored = "// authored hook must survive\nexport const handle = ({ event, resolve }) => resolve(event);\n"
			root, cfg := foreignFixture(t, `package data
import("context";"github.com/tylergannon/skgo")
func value(context.Context)(string,error){return "value",nil}
var _=skgo.Prerender(value)
`, map[string]string{"app/web/src/hooks.server." + extension: authored})
			if err := Run(cfg); err == nil || !strings.Contains(err.Error(), "is authored") {
				t.Fatalf("authored hook accepted: %v", err)
			}
			source, err := os.ReadFile(filepath.Join(root, "app", "web", "src", "hooks.server."+extension))
			if err != nil || string(source) != authored {
				t.Fatalf("authored hook overwritten: %s %v", source, err)
			}
		})
	}
}
