// Package example_test checks the two halves of the app against each other:
// the remote functions the Go binary serves, and the ones the built frontend
// calls. They are generated together and must never be allowed to drift apart
// unnoticed.
package example_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/generated"
	"github.com/tylergannon/skgo/example/web"
)

func buildManifest(t *testing.T) skgo.Manifest {
	t.Helper()
	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		t.Fatalf("opening the embedded build: %v", err)
	}
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		t.Fatalf("reading the build manifest: %v", err)
	}
	return manifest
}

func registryIDs(t *testing.T) []string {
	t.Helper()
	var ids []string
	for _, fn := range generated.Remotes() {
		ids = append(ids, fn.ID())
	}
	slices.Sort(ids)
	return ids
}

// TestTheBinaryAnswersEveryRemoteTheFrontendCalls is the startup check the
// server itself runs, applied to the artifacts in the tree.
func TestTheBinaryAnswersEveryRemoteTheFrontendCalls(t *testing.T) {
	manifest := buildManifest(t)
	built := slices.Clone(manifest.Remotes)
	slices.Sort(built)

	if got := registryIDs(t); !slices.Equal(got, built) {
		t.Fatalf("the Go registry serves\n  %v\nbut the built frontend calls\n  %v\nRun `go generate ./...` and rebuild the frontend.", got, built)
	}
}

// TestTheGeneratorAndTheBuildAgree closes the loop on the other side: the list
// the adapter copied into the build has to be the list `skgo generate` wrote.
func TestTheGeneratorAndTheBuildAgree(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("web", "skgo.remotes.json"))
	if err != nil {
		t.Fatalf("reading skgo.remotes.json: %v", err)
	}
	var declared struct {
		Remotes []string `json:"remotes"`
	}
	if err := json.Unmarshal(raw, &declared); err != nil {
		t.Fatalf("parsing skgo.remotes.json: %v", err)
	}
	slices.Sort(declared.Remotes)

	built := slices.Clone(buildManifest(t).Remotes)
	slices.Sort(built)
	if !slices.Equal(declared.Remotes, built) {
		t.Fatalf("skgo.remotes.json lists\n  %v\nbut the build carries\n  %v", declared.Remotes, built)
	}
}

// TestTheServerRefusesABuildItCannotServe proves the check is load-bearing:
// the same registry that starts against the real manifest refuses one that has
// drifted by a single function.
func TestTheServerRefusesABuildItCannotServe(t *testing.T) {
	manifest := buildManifest(t)
	if len(manifest.Remotes) == 0 {
		t.Fatal("the build lists no remote functions")
	}

	if _, err := skgo.NewRemotes(manifest.RemoteConfig("http://127.0.0.1:8080"), generated.Remotes()...); err != nil {
		t.Fatalf("the server refused the build it was compiled against: %v", err)
	}

	stale := manifest
	stale.Remotes = append(slices.Clone(manifest.Remotes)[1:], "deadbe/renamedInGo")
	_, err := skgo.NewRemotes(stale.RemoteConfig("http://127.0.0.1:8080"), generated.Remotes()...)
	if err == nil {
		t.Fatal("the server started against a frontend built from different remote functions")
	}
	t.Logf("refused, as it should: %v", err)
}

// serverLoadModules is the set of `+*.server.ts` modules the built frontend has
// a server load for: the key kit itself records for each node.
func serverLoadModules(t *testing.T) []string {
	t.Helper()
	var modules []string
	for _, module := range buildManifest(t).Nodes {
		if module != "" {
			modules = append(modules, module)
		}
	}
	slices.Sort(modules)
	return modules
}

// TestTheBinaryAnswersEveryServerLoadTheFrontendHas is the other half of the
// startup check. Kit decides whether its client ever asks for `__data.json`
// from the `load` export it finds in the built module, so a page whose stub
// reached the build without a Go load behind it is a page nobody answers.
func TestTheBinaryAnswersEveryServerLoadTheFrontendHas(t *testing.T) {
	var registered []string
	for _, load := range generated.Loads() {
		registered = append(registered, load.Module())
	}
	slices.Sort(registered)

	if built := serverLoadModules(t); !slices.Equal(registered, built) {
		t.Fatalf("the Go registry answers\n  %v\nbut the built frontend has server loads at\n  %v\nRun `go generate ./...` and rebuild the frontend.", registered, built)
	}
}

// TestTheGeneratorAndTheBuildAgreeAboutLoads closes the loop on the other side.
func TestTheGeneratorAndTheBuildAgreeAboutLoads(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("web", "skgo.remotes.json"))
	if err != nil {
		t.Fatalf("reading skgo.remotes.json: %v", err)
	}
	var declared struct {
		Loads []string `json:"loads"`
	}
	if err := json.Unmarshal(raw, &declared); err != nil {
		t.Fatalf("parsing skgo.remotes.json: %v", err)
	}
	slices.Sort(declared.Loads)

	if built := serverLoadModules(t); !slices.Equal(declared.Loads, built) {
		t.Fatalf("skgo.remotes.json lists\n  %v\nbut the build carries\n  %v", declared.Loads, built)
	}
}

// TestTheServerRefusesABuildWhoseLoadsItCannotAnswer proves that check is
// load-bearing too.
func TestTheServerRefusesABuildWhoseLoadsItCannotAnswer(t *testing.T) {
	manifest := buildManifest(t)
	if len(serverLoadModules(t)) == 0 {
		t.Fatal("the build has no server loads")
	}

	if _, err := skgo.NewLoads(manifest.LoadConfig("http://127.0.0.1:8080"), generated.Loads()...); err != nil {
		t.Fatalf("the server refused the build it was compiled against: %v", err)
	}

	stale := manifest
	stale.Nodes = slices.Clone(manifest.Nodes)
	for i, module := range stale.Nodes {
		if module != "" {
			stale.Nodes[i] = "src/routes/renamed/+page.server.ts"
			break
		}
	}
	_, err := skgo.NewLoads(stale.LoadConfig("http://127.0.0.1:8080"), generated.Loads()...)
	if err == nil {
		t.Fatal("the server started against a frontend whose server loads it does not answer")
	}
	t.Logf("refused, as it should: %v", err)
}
