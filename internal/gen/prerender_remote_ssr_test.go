package gen

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCompiledProductionNoArgumentNullReachesTheArtifactLookup builds the
// production Goja bundle from an isolated authored remote. Kit's production
// wrapper must disable the no-argument validator before the invocation reaches
// Go; the missing null-key artifact then answers 500 without running the body.
func TestCompiledProductionNoArgumentNullReachesTheArtifactLookup(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	app, err := copyExample(root, filepath.Join(t.TempDir(), "example"))
	if err != nil {
		t.Fatal(err)
	}
	ui := filepath.Join(app, "ui")
	if err := os.Rename(filepath.Join(app, "web"), ui); err != nil {
		t.Fatal(err)
	}
	if err := replaceOnce(filepath.Join(app, "internal", "skgo", "config.go"), "--web ../../web", "--web ../../ui"); err != nil {
		t.Fatal(err)
	}
	if err := replaceOnce(filepath.Join(app, "go.mod"), "ignore ./web/node_modules", "ignore ./ui/node_modules"); err != nil {
		t.Fatal(err)
	}
	if err := replaceOnce(filepath.Join(app, "cmd", "main.go"), "github.com/tylergannon/skgo/example/web", "github.com/tylergannon/skgo/example/ui"); err != nil {
		t.Fatal(err)
	}
	linkExampleFrontendDependencies(t, root, app)
	write := func(name, content string) {
		t.Helper()
		file := filepath.Join(ui, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := replaceOnce(filepath.Join(app, "server.go"),
		"skgo.Sequence(skgo.Handle(Handle).Middleware(), SerializedHeaders, VisitMiddleware).Intercept",
		"skgo.Sequence(skgo.Handle(Handle).Middleware(), SerializedHeaders, VisitMiddleware, PrerenderTransformFixture).Intercept"); err != nil {
		t.Fatalf("install isolated transform middleware in copied app: %v", err)
	}
	if err := os.WriteFile(filepath.Join(app, "prerender_transform_fixture.go"), []byte(`package example

import "github.com/tylergannon/skgo"

var PrerenderTransformFixture skgo.Middleware
`), 0o644); err != nil {
		t.Fatalf("write isolated transform middleware slot: %v", err)
	}
	write("src/routes/prerender-contract/fixture.remote.go", `package prerendercontract

import (
	"context"
	"sync/atomic"

	"github.com/tylergannon/skgo"
)

var calls atomic.Int32

func noargValue(context.Context) (string, error) {
	calls.Add(1)
	return "live no-argument body ran", nil
}

func Calls() int32 { return calls.Load() }

var _ = skgo.Prerender(noargValue)
`)
	write("src/routes/prerender-contract/+page.svelte", `<script>import { noargValue } from './fixture.remote';const value=await noargValue(null);</script><h1>Prerender contract</h1><p>{value}</p>`)
	write("src/routes/prerender-contract/+error.svelte", `<script>let { error, status } = $props();</script><h1>{status}</h1><p>{error.message}</p>`)
	write("src/routes/prerender-contract-seed/+page.ts", `import { noargValue } from '../prerender-contract/fixture.remote';

export const prerender = true;

export async function load() {
	return { value: await noargValue() };
}
`)
	write("src/routes/prerender-contract-seed/+page.svelte", `<script>let { data } = $props();</script><h1>Prerender seed</h1><p>{data.value}</p>`)
	write("src/routes/prerender-contract-error/+page.ts", `import { noargValue } from '../prerender-contract/fixture.remote';

export async function load() {
	return { value: await noargValue() };
}
`)
	write("src/routes/prerender-contract-error/+page.svelte", `<script>let { data } = $props();</script><h1>Prerender error consumer</h1><p>{data.value}</p>`)
	write("src/routes/prerender-contract-transform/+page.ts", `import { noargValue } from '../prerender-contract/fixture.remote';

export const prerender = false;

export async function load() {
	return { value: await noargValue() };
}
`)
	write("src/routes/prerender-contract-transform/+page.svelte", `<script>let { data } = $props();</script><h1>Prerender transform consumer</h1><p>{data.value}</p>`)
	write("src/routes/prerender-contract-missing/+page.ts", `import { noargValue } from '../prerender-contract/fixture.remote';

export async function load() {
	// @ts-expect-error exercise the production null-key artifact miss
	return { value: await noargValue(null) };
}
`)
	write("src/routes/prerender-contract-missing/+page.svelte", `<script>let { data } = $props();</script><h1>Prerender missing consumer</h1><p>{data.value}</p>`)
	if output, err := runGoGenerate(app); err != nil {
		t.Fatalf("go generate ./...: %v\n%s", err, output)
	}
	cmd := exec.Command(filepath.Join(ui, "node_modules", ".bin", "vp"), "build")
	cmd.Dir = ui
	cmd.Env = append(os.Environ(), "GOWORK=off", "ORIGIN=http://127.0.0.1:8080")
	output, err := cmd.CombinedOutput()
	t.Logf("vp build exit: %d\n%s", exitCode(err), output)
	if err != nil {
		t.Fatalf("vp build: %v", err)
	}
	var noargArtifacts []string
	if err := filepath.WalkDir(filepath.Join(ui, "build"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Base(path) != "noargValue" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "live no-argument body ran") {
			noargArtifacts = append(noargArtifacts, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("locate native noargValue result artifact: %v", err)
	}
	if len(noargArtifacts) != 1 {
		t.Fatalf("found %d noargValue result artifacts, want exactly one: %v", len(noargArtifacts), noargArtifacts)
	}
	nativeRemoteID, err := filepath.Rel(filepath.Join(ui, "build", "prerendered", "_app", "remote"), noargArtifacts[0])
	if err != nil {
		t.Fatalf("locate native remote ID from its artifact path: %v", err)
	}
	nativeRemoteID = filepath.ToSlash(nativeRemoteID)
	nativeArtifactPath, err := filepath.Rel(filepath.Join(ui, "build"), noargArtifacts[0])
	if err != nil {
		t.Fatalf("locate native artifact in build FS: %v", err)
	}
	nativeArtifactPath = filepath.ToSlash(nativeArtifactPath)
	const builtError = `{"type":"error","error":{"status":409,"message":"built no-argument failure","marker":"native-prerender-error"}}`
	if err := os.WriteFile(noargArtifacts[0], []byte(builtError), 0o644); err != nil {
		t.Fatalf("replace native noargValue result with recorded error: %v", err)
	}
	fixture := filepath.Join(app, "cmd", "prerender-fixture")
	if err := os.MkdirAll(fixture, 0o755); err != nil {
		t.Fatal(err)
	}
	fixtureTest := `package prerenderfixture_test

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example"
	prerendercontract "github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf5yhezlsmvxgizlsfvrw63tuojqwg5a"
	ui "github.com/tylergannon/skgo/example/ui"
)

type oneFileOverlay struct {
	fs.FS
	name string
	data []byte
}

func (o oneFileOverlay) Open(name string) (fs.File, error) {
	if name == o.name {
		return (fstest.MapFS{"artifact": {Data: o.data}}).Open("artifact")
	}
	return o.FS.Open(name)
}

func TestProductionNoArgumentNullUsesArtifactMiss(t *testing.T) {
	dist, err := fs.Sub(ui.Build, "build")
	if err != nil { t.Fatal(err) }
	handler, mode, err := example.NewHandler(dist, "", "http://127.0.0.1:8080")
	if err != nil { t.Fatal(err) }
	if mode != "prod" { t.Fatalf("mode = %q", mode) }
	remoteID := "__NATIVE_REMOTE_ID__"
	nativeArtifactPath := "__NATIVE_ARTIFACT_PATH__"
	assertPEntry := func(document, key string, required ...string) {
		t.Helper()
		marker := "\"" + key + "\":"
		start := strings.Index(document, marker)
		if start < 0 {
			t.Fatalf("document omitted original p answer %q: %s", key, document)
		}
		tail := document[start:]
		if end := strings.Index(tail, "}}"); end >= 0 {
			tail = tail[:end+2]
		}
		for _, text := range required {
			if !strings.Contains(tail, text) {
				t.Fatalf("p answer %q omitted %q: %s", key, text, document)
			}
		}
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/prerender-contract", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want opaque 500 (a 400 means Kit validated before artifact lookup): %s", recorder.Code, recorder.Body.String())
	}
	if got := prerendercontract.Calls(); got != 0 {
		t.Fatalf("the Go body ran %d times for the missing null-key artifact", got)
	}
	if strings.Contains(recorder.Body.String(), "live no-argument body ran") {
		t.Fatal("the missing artifact fell back to the Go body")
	}
	assertPEntry(recorder.Body.String(), remoteID+"/W251bGxd", "message:\"Internal Error\"", "status:500")
	missingRecorder := httptest.NewRecorder()
	handler.ServeHTTP(missingRecorder, httptest.NewRequest(http.MethodGet, "/prerender-contract-missing", nil))
	if missingRecorder.Code != http.StatusInternalServerError {
		t.Fatalf("universal-load missing null-key status = %d, want opaque 500: %s", missingRecorder.Code, missingRecorder.Body.String())
	}
	assertPEntry(missingRecorder.Body.String(), remoteID+"/W251bGxd", "message:\"Internal Error\"", "status:500")
	errorRecorder := httptest.NewRecorder()
	handler.ServeHTTP(errorRecorder, httptest.NewRequest(http.MethodGet, "/prerender-contract-error", nil))
	errorHTML := errorRecorder.Body.String()
	if errorRecorder.Code != 409 || !strings.Contains(errorHTML, "built no-argument failure") || !strings.Contains(errorHTML, "native-prerender-error") {
		t.Fatalf("handled remote error document status=%d body=%s", errorRecorder.Code, errorHTML)
	}
	assertPEntry(errorHTML, remoteID+"/", "status:409", "message:\"built no-argument failure\"", "marker:\"native-prerender-error\"")
	if got := prerendercontract.Calls(); got != 0 {
		t.Fatalf("the Go body ran %d times while serving the recorded error", got)
	}
	transformFS := oneFileOverlay{
		FS:   dist,
		name: nativeArtifactPath,
		data: []byte("{\"type\":\"result\",\"data\":\"[{\\\"_\\\":1},\\\"built transform value\\\"]\"}"),
	}
	transformCalls := 0
	example.PrerenderTransformFixture = skgo.Middleware(func(ctx context.Context, _ *skgo.Event, resolve skgo.Resolve) (*http.Response, error) {
		return resolve(ctx, skgo.ResolveOptions{TransformPageChunk: func(_ context.Context, html string, _ bool) (string, error) {
			transformCalls++
			if transformCalls == 1 {
				return "", errors.New("one-shot transform failure")
			}
			return html, nil
		}})
	})
	transformHandler, transformMode, err := example.NewHandler(transformFS, "", "http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	if transformMode != "prod" {
		t.Fatalf("transform handler mode = %q", transformMode)
	}
	transformRecorder := httptest.NewRecorder()
	transformHandler.ServeHTTP(transformRecorder, httptest.NewRequest(http.MethodGet, "/prerender-contract-transform", nil))
	transformHTML := transformRecorder.Body.String()
	if transformRecorder.Code != http.StatusInternalServerError || transformCalls != 2 {
		t.Fatalf("transform fallback status=%d calls=%d body=%s", transformRecorder.Code, transformCalls, transformHTML)
	}
	assertPEntry(transformHTML, remoteID+"/", "built transform value")
	if got := prerendercontract.Calls(); got != 0 {
		t.Fatalf("the Go body ran %d times while serving the transform error", got)
	}
}
`
	fixtureTest = strings.Replace(fixtureTest, "__NATIVE_REMOTE_ID__", nativeRemoteID, 1)
	fixtureTest = strings.Replace(fixtureTest, "__NATIVE_ARTIFACT_PATH__", nativeArtifactPath, 1)
	if err := os.WriteFile(filepath.Join(fixture, "ssr_test.go"), []byte(fixtureTest), 0o644); err != nil {
		t.Fatal(err)
	}
	goTest := exec.Command("go", "test", "./cmd/prerender-fixture", "-run", "^TestProductionNoArgumentNullUsesArtifactMiss$", "-count=1", "-v")
	goTest.Dir = app
	goTest.Env = append(os.Environ(), "GOWORK=off")
	testOutput, err := goTest.CombinedOutput()
	t.Logf("fixture Go SSR test exit: %d\n%s", exitCode(err), testOutput)
	if err != nil {
		t.Fatalf("compiled production SSR fixture: %v", err)
	}
}
