package devrender_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example"
)

func TestDevSSRLoadsOnlyRequestedNodes(t *testing.T) {
	const first = "/about"
	const other = "/empty"
	await := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for !check() {
			if time.Now().After(deadline) {
				t.Fatal("dev change did not become visible")
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	changes := func(since int) (int, []string) {
		t.Helper()
		response, err := http.Get(devServer + "/__skgo_dev/changed?since=" + strconv.Itoa(since))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var out struct {
			Version int
			Files   []string
		}
		if err := json.NewDecoder(response.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out.Version, out.Files
	}
	saved := map[string]bool{}
	write := func(route, file, source string) {
		t.Helper()
		dir := filepath.Join(webRoot, "src/routes", route)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, file)
		if !saved[path] {
			before, err := os.ReadFile(path)
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			existed := err == nil
			t.Cleanup(func() {
				if existed {
					os.WriteFile(path, before, 0644)
				} else {
					os.Remove(path)
				}
			})
			saved[path] = true
		}
		since, _ := changes(0)
		if err := os.WriteFile(path, []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil {
			t.Fatal(err)
		}
		await(func() bool {
			_, files := changes(since)
			for _, changed := range files {
				if changed == path || changed == canonical {
					return true
				}
			}
			return false
		})
	}

	write(first, "+page.svelte", `<script>let { data } = $props();</script><h1>FIRST_ROUTE_41</h1><p>{data.message}</p>`)
	write(first, "+page.ts", `export function load() { return {message: 'UNIVERSAL_41'}; }`)
	write(other, "+page.svelte", `<h1>UNVISITED_ROUTE_41</h1>`)
	await(func() bool {
		m, e := skgo.ReadDevManifest(dist(), devServer)
		if e != nil {
			return false
		}
		for _, r := range m.Routes {
			if r.ID == other {
				return true
			}
		}
		return false
	})
	target, err := url.Parse(devServer)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	var mu sync.Mutex
	var modules []string
	observed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__skgo_dev/module" {
			b, e := io.ReadAll(r.Body)
			if e != nil {
				t.Error(e)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(b))
			var request struct{ URL string }
			if e = json.Unmarshal(b, &request); e != nil {
				t.Error(e)
				return
			}
			mu.Lock()
			modules = append(modules, request.URL)
			mu.Unlock()
		}
		proxy.ServeHTTP(w, r)
	}))
	defer observed.Close()
	snapshot := func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), modules...) }
	start := time.Now()
	h, _, err := example.NewHandler(dist(), observed.URL, origin)
	if err != nil {
		t.Fatal(err)
	}
	startup := snapshot()
	t.Logf("startup: %d module requests in %s", len(startup), time.Since(start))
	getPage := func(route string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", route, nil)
		req.Header.Set("Accept", "text/html")
		h.ServeHTTP(rec, req)
		return rec
	}
	expect := func(route string, status int, text string) {
		t.Helper()
		rec := getPage(route)
		if rec.Code != status || !strings.Contains(rec.Body.String(), text) {
			t.Fatalf("GET %s: status %d, want %d and literal %q:\n%s", route, rec.Code, status, text, rec.Body.String())
		}
	}
	expect(first, 200, "UNIVERSAL_41")
	t.Logf("first page: %d additional module requests", len(snapshot())-len(startup))
	for _, name := range startup {
		if strings.Contains(name, "/src/routes"+first+"/") || strings.Contains(name, "/src/routes"+other+"/") {
			t.Fatalf("startup fetched unrelated page %s", name)
		}
	}
	for _, name := range snapshot() {
		if strings.Contains(name, "/src/routes"+other+"/") {
			t.Fatalf("first page fetched unrelated page %s", name)
		}
	}
	// A compile error in an unvisited route must not break this page or startup.
	write(other, "+page.svelte", `<script>let broken = ;</script><h1>UNVISITED_BROKEN</h1>`)
	expect(first, 200, "FIRST_ROUTE_41")
	for _, name := range snapshot() {
		if strings.Contains(name, "/src/routes"+other+"/") {
			t.Fatalf("unvisited edit loaded unrelated page %s", name)
		}
	}
	await(func() bool { rec := getPage(other); return rec.Code == 500 })
	expect(first, 200, "FIRST_ROUTE_41")
	write(other, "+page.svelte", `<h1>UNVISITED_REPAIRED_43</h1>`)
	await(func() bool {
		rec := getPage(other)
		return rec.Code == 200 && strings.Contains(rec.Body.String(), "UNVISITED_REPAIRED_43")
	})
	// Both the component and its separately loaded universal module invalidate.
	write(first, "+page.svelte", `<script>let { data } = $props();</script><h1>VISITED_EDIT_44</h1><p>{data.message}</p>`)
	write(first, "+page.ts", `export function load() { return {message: 'UNIVERSAL_45'}; }`)
	await(func() bool {
		rec := getPage(first)
		return rec.Code == 200 && strings.Contains(rec.Body.String(), "VISITED_EDIT_44") && strings.Contains(rec.Body.String(), "UNIVERSAL_45")
	})
	// A new layout changes the node table without changing generated caller routes.
	write(first, "+layout.svelte", `<script>let { children } = $props();</script><section>NEW_NODE_46{@render children()}</section>`)
	await(func() bool {
		rec := getPage(first)
		return rec.Code == 200 && strings.Contains(rec.Body.String(), "NEW_NODE_46") && strings.Contains(rec.Body.String(), "VISITED_EDIT_44")
	})
	expect(other, 200, "UNVISITED_REPAIRED_43")
	if err := os.Remove(filepath.Join(webRoot, "src/routes", first, "+layout.svelte")); err != nil {
		t.Fatal(err)
	}
	await(func() bool {
		rec := getPage(first)
		return rec.Code == 200 && !strings.Contains(rec.Body.String(), "NEW_NODE_46") && strings.Contains(rec.Body.String(), "UNIVERSAL_45")
	})
}
