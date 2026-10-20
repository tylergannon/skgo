//go:build skgo_qualification

package example_test

import (
	"cmp"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/tylergannon/skgo/internal/clienthandoff"
)

// TestBuiltClientHandoff runs example/web/tests/client-handoff.test.ts against
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
	web, err := filepath.Abs("web")
	if err != nil {
		t.Fatal(err)
	}
	output, err := clienthandoff.Run(t, web, origin, "production")
	if err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
	t.Logf("%s", output)
}
