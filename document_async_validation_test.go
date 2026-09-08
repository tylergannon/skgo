package skgo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/ssr"
)

// TestConcurrentRemoteCallsMergeIntoTheSameAnswersMapSafely exercises the
// production path internal/ssr's own tests cannot: renderPlan's real Remote
// host wraps s.answer/s.record, which mutate a shared Go map (document.go's
// `into[kind][key] = answer`), and independent host calls genuinely run on
// separate goroutines (proven below with a three-way gate, the same pattern
// internal/ssr/async_test.go uses to prove overlap). Without renderPlan's
// per-call scratch map plus mutex-guarded merge, this is a concurrent map
// write. Run with -race; it must find nothing, and all three answers must
// still land in the merged map.
func TestConcurrentRemoteCallsMergeIntoTheSameAnswersMapSafely(t *testing.T) {
	var started atomic.Int32
	gate := make(chan struct{})
	arrive := func() {
		if started.Add(1) == 3 {
			close(gate)
		}
		<-gate
	}
	amber := NewQueryNoArg("src/lib/colors.remote.ts", "amber", func(context.Context) (string, error) {
		arrive()
		return "Amber", nil
	})
	birch := NewQueryNoArg("src/lib/colors.remote.ts", "birch", func(context.Context) (string, error) {
		arrive()
		return "Birch", nil
	})
	cobalt := NewQueryNoArg("src/lib/colors.remote.ts", "cobalt", func(context.Context) (string, error) {
		arrive()
		return "Cobalt", nil
	})

	rs, err := NewRemotes(RemoteConfig{Version: "v1"}, amber, birch, cobalt)
	if err != nil {
		t.Fatal(err)
	}

	ids, err := json.Marshal([]string{amber.ID(), birch.ID(), cobalt.ID()})
	if err != nil {
		t.Fatal(err)
	}
	bundle := fmt.Sprintf(`
globalThis.__skgo_ping = () => 'ok';
globalThis.__skgo_render = (json) => {
  const result = {done:false};
  Promise.all(%s.map(id => __skgo_remote(id, ''))).then(function(values) {
    result.body = String(values.length);
    result.done = true;
  }, function(e) { result.failure = String(e); result.done = true; });
  return result;
};`, ids)

	engine, err := ssr.New("concurrent.js", []byte(bundle), 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	s := &SSR{remotes: rs, loads: &Loads{}, engine: engine}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r := httptest.NewRequest("GET", "/concurrent", nil).WithContext(ctx)
	req := dataRequest{url: mustURL(t, "http://127.0.0.1/concurrent")}
	plan := documentPlan{routeID: "/concurrent", params: map[string]string{}, status: 200}
	csp := newDocumentCSPWithNonce(nil, "test-nonce")

	result, answers, err := s.renderPlan(r, req, plan, csp)
	if err != nil {
		t.Fatal(err)
	}
	if result.Body != "3" {
		t.Fatalf("body = %q, want %q", result.Body, "3")
	}

	q := answers["q"]
	if len(q) != 3 {
		t.Fatalf("answers[\"q\"] has %d entries, want 3: %#v", len(q), q)
	}
	for _, tc := range []struct {
		fn   *Remote
		want string
	}{{amber, "Amber"}, {birch, "Birch"}, {cobalt, "Cobalt"}} {
		entry, ok := q[tc.fn.ID()+"/"]
		if !ok {
			t.Fatalf("no answer recorded for %s", tc.fn.ID())
		}
		if entry.tree != tc.want {
			t.Fatalf("%s tree = %#v, want %q", tc.fn.ID(), entry.tree, tc.want)
		}
	}
}
