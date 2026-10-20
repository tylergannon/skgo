//go:build skgo_dev_integration

package devserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// A route and layout edit lands while a development page's Go load is held on
// network I/O. Other Go endpoints keep answering, the held page finishes with
// its Go data, and the next request renders the new route.
func TestRealSessionRouteEditDuringPendingLoad(t *testing.T) {
	realSession(t, false, false, false, "pending-edit")
}

// pendingGate is the network response the held load waits for.
type pendingGate struct {
	entered, released chan struct{}
	release           func()
}

func setupPendingEdit(t *testing.T, write func(string, string)) *pendingGate {
	t.Helper()
	gate := &pendingGate{entered: make(chan struct{}), released: make(chan struct{})}
	var enterOnce, releaseOnce sync.Once
	gate.release = func() { releaseOnce.Do(func() { close(gate.released) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enterOnce.Do(func() { close(gate.entered) })
		select {
		case <-gate.released:
			io.WriteString(w, "literal-go-released-113")
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(gate.release)
	write("web/src/routes/pending/+page.svelte", `<script>let {data}=$props();</script><h1>literal-held-page-113</h1><p data-testid="go-receipt">{data.receipt}</p>`)
	write("web/src/routes/pending/page.server.go", fmt.Sprintf(`package pending
import("io";"net/http";"github.com/tylergannon/skgo")
type PageData struct{Receipt string `+"`json:\"receipt\"`"+`}
func load(event PageRequestEvent)(PageData,error){
 request,err:=http.NewRequestWithContext(event.Request().Context(),"GET",%q,nil);if err!=nil{return PageData{},err}
 response,err:=http.DefaultClient.Do(request);if err!=nil{return PageData{},err};defer response.Body.Close()
 receipt,err:=io.ReadAll(response.Body);return PageData{Receipt:string(receipt)},err
}
var _=skgo.Load(load)
`, server.URL))
	return gate
}

func exercisePendingEdit(t *testing.T, app, origin string, log *sessionLog, gate *pendingGate) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	type page struct {
		status int
		body   string
		err    error
	}
	held := make(chan page, 1)
	go func() {
		request, err := http.NewRequestWithContext(ctx, "GET", origin+"/pending", nil)
		if err != nil {
			held <- page{err: err}
			return
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			held <- page{err: err}
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		held <- page{status: response.StatusCode, body: string(body), err: err}
	}()
	select {
	case <-gate.entered:
	case p := <-held:
		t.Fatalf("page finished before its load reached Go I/O: %d %v %s\n%s", p.status, p.err, p.body, log.String())
	case <-ctx.Done():
		t.Fatalf("load never reached Go I/O\n%s", log.String())
	}

	response, err := (&http.Client{Timeout: 10 * time.Second}).Get(origin + "/orders")
	if err != nil {
		t.Fatalf("independent endpoint: %v\n%s", err, log.String())
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || string(body) != "literal-4101:8" {
		t.Fatalf("independent endpoint while the load waits: %d %q\n%s", response.StatusCode, body, log.String())
	}

	for name, source := range map[string]string{
		"web/src/routes/pending/+layout.svelte":       `<script>let {children}=$props();</script><section data-testid="edited-layout"><h2>literal-new-layout-113</h2>{@render children()}</section>`,
		"web/src/routes/pending/changed/+page.svelte": `<h1>literal-new-route-113</h1>`,
	} {
		path := filepath.Join(app, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case p := <-held:
		t.Fatalf("page finished before its load was released: %d %v %s\n%s", p.status, p.err, p.body, log.String())
	default:
	}

	gate.release()
	p := <-held
	if p.err != nil || p.status != 200 || !strings.Contains(p.body, "literal-held-page-113") || !strings.Contains(p.body, `<p data-testid="go-receipt">literal-go-released-113</p>`) {
		t.Fatalf("held page: %d %v\n%s\n%s", p.status, p.err, p.body, log.String())
	}

	// Kit debounces route manifest updates, so the new route appears on a
	// later request, as it would for a developer's next reload.
	deadline := time.Now().Add(20 * time.Second)
	last := ""
	for time.Now().Before(deadline) {
		response, err := http.Get(origin + "/pending/changed")
		if err == nil {
			body, _ := io.ReadAll(response.Body)
			response.Body.Close()
			last = fmt.Sprintf("%d %s", response.StatusCode, body)
			if response.StatusCode == 200 && strings.Contains(string(body), `<section data-testid="edited-layout"><h2>literal-new-layout-113</h2>`) && strings.Contains(string(body), "<h1>literal-new-route-113</h1>") {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("changed route never rendered with its new layout; last: %s\n%s", last, log.String())
}
