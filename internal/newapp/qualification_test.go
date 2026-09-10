//go:build qualification

package newapp_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/newapp"
)

// TestGeneratedProjectBrowserContract qualifies the actual candidate artifacts
// through the browser contract emitted by a completely fresh project. It is a
// release test, not part of the pull-request suite: run it explicitly with
// `go test -tags=qualification ./internal/newapp -run TestGeneratedProjectBrowserContract`.
func TestGeneratedProjectBrowserContract(t *testing.T) {
	root := t.TempDir()
	proxy := publish(t, filepath.Join(root, "proxy"))
	dir := filepath.Join(root, "release-candidate")
	port := freePort(t)
	origin := fmt.Sprintf("http://127.0.0.1:%d", port)
	adapter := packAdapter(t, root)

	if err := newapp.Create(newapp.Options{
		Dir:         dir,
		App:         "release-candidate",
		Origin:      origin,
		SkgoVersion: scaffoldVersion,
		AdapterSpec: "file:" + adapter,
		Logf:        t.Logf,
	}); err != nil {
		t.Fatalf("scaffolding the release candidate: %v", err)
	}

	env := append(os.Environ(),
		"GOPROXY=file://"+filepath.ToSlash(proxy)+",https://proxy.golang.org,direct",
		"GOPRIVATE=",
		"GONOPROXY=none",
		"GONOSUMDB=none",
		"GOSUMDB=off",
		"GOFLAGS=-mod=mod",
		"GOWORK=off",
	)
	run(t, dir, env, "mise", "trust", "--yes")
	run(t, dir, env, "mise", "run", "build")

	client := &http.Client{Timeout: 20 * time.Second}
	artifacts := qualificationArtifacts(t)
	binary := filepath.Join(dir, "bin", "release-candidate")

	stopProduction := serveBinary(t, binary, port)
	awaitServer(t, client, origin)
	runGeneratedBDD(t, dir, env, origin, "prod", artifacts)
	stopProduction()

	vitePort := freePort(t)
	viteOrigin := fmt.Sprintf("http://127.0.0.1:%d", vitePort)
	serveVite(t, filepath.Join(dir, "web"), env, vitePort)
	awaitPort(t, vitePort)
	stopDevelopment := serveBinary(t, binary, port, "--proxy", viteOrigin, "--origin", origin)
	awaitServer(t, client, origin)
	runGeneratedBDD(t, dir, env, origin, "dev", artifacts)
	stopDevelopment()
}

func runGeneratedBDD(t *testing.T, dir string, env []string, origin, mode, artifacts string) {
	t.Helper()
	run(t, dir, append(env,
		"BASE_URL="+origin,
		"SKGO_E2E_RUN="+mode,
		"SKGO_E2E_ARTIFACTS="+artifacts,
	), "mise", "run", "e2e")
}

func qualificationArtifacts(t *testing.T) string {
	t.Helper()
	root := os.Getenv("SKGO_QUALIFICATION_ARTIFACTS")
	if root == "" {
		return filepath.Join(t.TempDir(), "generated")
	}
	at := filepath.Join(root, "generated")
	if err := os.MkdirAll(at, 0o755); err != nil {
		t.Fatal(err)
	}
	return at
}
