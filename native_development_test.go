package skgo

import (
	"context"
	"fmt"
	"github.com/tylergannon/skgo/internal/adapter"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// A filesystem decorator need only implement fs.FS. Development selection is
// explicit and survives ordinary Go composition without forwarding hidden types.
type developmentBuildDecorator struct{ fs.FS }

func TestNativeDevelopmentComposesThroughOrdinaryFilesystem(t *testing.T) {
	build := developmentBuildDecorator{fstest.MapFS{
		"dist/skgo.manifest.json": &fstest.MapFile{Data: []byte(fmt.Sprintf(`{"version":"literal-416","skgoAdapter":%q,"routes":[]}`, adapter.Fingerprint()))},
	}}
	dist, err := fs.Sub(build, "dist")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := ReadManifest(dist)
	if err != nil {
		t.Fatal(err)
	}
	development := NewNativeDevelopment("composition-813")
	loads, err := NewLoads(LoadConfig{Origin: "http://app.test"})
	if err != nil {
		t.Fatal(err)
	}
	remotes, err := NewRemotes(RemoteConfig{Origin: "http://app.test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = development.NewSSR(dist, manifest, loads, remotes, SSROptions{}); err != nil {
		t.Fatal(err)
	}
	pages, err := development.NewStaticHandler(dist)
	if err != nil {
		t.Fatal(err)
	}
	handler, close, err := development.Handler(pages)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	rr := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		w.Write([]byte("decorated-build-916"))
	})
	request := httptest.NewRequest("GET", "http://app.test/page", nil)
	handler.ServeHTTP(rr, request.WithContext(context.WithValue(request.Context(), devContinuationKey{}, next)))
	if rr.Code != 201 || rr.Body.String() != "decorated-build-916" {
		t.Fatalf("got %d %q", rr.Code, rr.Body.String())
	}
	// A production constructor cannot silently select development from a special
	// filesystem type or from the presence of a different development instance.
	if _, err = NewSSR(dist, manifest, loads, remotes, SSROptions{}); err == nil {
		t.Fatal("production build without SSR bundle unexpectedly accepted")
	}
}
