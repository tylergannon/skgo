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
