package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile("handler.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func edited(body []byte, name string, factor int) []byte {
	result := strings.ReplaceAll(string(body), "original", name)
	return []byte(strings.ReplaceAll(result, "n * 3", fmt.Sprintf("n * %d", factor)))
}

func get(t *testing.T, url string) string {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Header.Get("X-Host-PID") != strconv.Itoa(os.Getpid()) {
		t.Fatal("host PID changed")
	}
	return string(body)
}

func mustGet(t *testing.T, url, want string) {
	t.Helper()
	if got := get(t, url); got != want {
		t.Fatalf("HTTP = %q; want %q", got, want)
	}
}

// Read the loader's actual mmap bases, not its bookkeeping counters. This is
// test-only inspection of the pinned upstream representation.
func mappedPages(r *reloader) [2]uintptr {
	module := reflect.ValueOf(r.current.module).Elem()
	return [2]uintptr{module.FieldByName("codeByte").Pointer(), module.FieldByName("dataByte").Pointer()}
}

func assertMapped(t *testing.T, pages [2]uintptr, want bool) {
	t.Helper()
	for _, base := range pages {
		mapped := mappedAt(base)
		if mapped < 0 {
			t.Fatalf("OS mapping probe failed at %#x", base)
		}
		if (mapped == 1) != want {
			t.Fatalf("mapping %#x: mapped=%t want=%t", base, mapped == 1, want)
		}
	}
}

func TestNativeLifecycle(t *testing.T) {
	r, err := newReloader(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	source := fixture(t)
	if _, err := r.Reload(source); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(r)
	defer server.Close()
	mustGet(t, server.URL+"/proof?n=7", "original:1:21:/proof")
	// Hold an HTTP request inside compiled code while the next revision loads.
	entered, release := make(chan struct{}), make(chan struct{})
	r.pause = func() { close(entered); <-release }
	held := make(chan string, 1)
	go func() { held <- get(t, server.URL+"/hold?n=5") }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("old handler did not enter")
	}
	candidateReady := make(chan struct{})
	r.beforeDrain = func() { close(candidateReady) }
	swapped := make(chan error, 1)
	go func() { _, err := r.Reload(edited(source, "revised", 5)); swapped <- err }()
	select {
	case <-candidateReady:
	case <-time.After(5 * time.Second):
		t.Fatal("candidate did not reach drain")
	}
	select {
	case err := <-swapped:
		t.Fatalf("unloaded active handler: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	runtime.GC()
	close(release)
	if got := <-held; got != "original:2:15:/hold" {
		t.Fatalf("held response = %q", got)
	}
	if err := <-swapped; err != nil {
		t.Fatal(err)
	}
	r.beforeDrain = nil
	mustGet(t, server.URL+"/proof?n=7", "revised:3:35:/proof")
	// Compiler failure must preserve the exact previous handler and host counter.
	broken := strings.ReplaceAll(string(edited(source, "broken", 99)), "n * 99", "missingIdentifier")
	if _, err := r.Reload([]byte(broken)); err == nil || errors.Is(err, ErrRestart) {
		t.Fatalf("want compile failure, got %v", err)
	}
	mustGet(t, server.URL+"/proof?n=7", "revised:4:35:/proof")
	if _, err := r.Reload(edited(source, "repaired", 11)); err != nil {
		t.Fatal(err)
	}
	mustGet(t, server.URL+"/proof?n=7", "repaired:5:77:/proof")
	// A literal returned by old native code is still readable after its module
	// has been unmapped and garbage collection has run.
	retained := r.response(httptest.NewRequest("GET", "/literal", nil))
	if _, err := r.Reload(edited(source, "after-literal", 13)); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	if retained != "repaired-literal" {
		t.Fatalf("retained literal = %q", retained)
	}
	mustGet(t, server.URL+"/proof?n=7", "after-literal:6:91:/proof")
	for name, edit := range incompatible(source) {
		if _, err := r.Reload(edit); !errors.Is(err, ErrRestart) {
			t.Fatalf("%s: want restart, got %v", name, err)
		}
	}
	mustGet(t, server.URL+"/proof?n=7", "after-literal:7:91:/proof")
	// Reverting source is another compile, never an expired cached revision.
	if _, err := r.Reload(source); err != nil {
		t.Fatal(err)
	}
	mustGet(t, server.URL+"/proof?n=7", "original:8:21:/proof")
	r.Close()
	if r.loaded != 5 || r.unloaded != 5 || r.live != 0 || r.peak != 2 {
		t.Fatalf("lifetime counts loaded=%d unloaded=%d live=%d peak=%d", r.loaded, r.unloaded, r.live, r.peak)
	}
	t.Logf("pid=%d: exact HTTP edits, retained counter, held request, compile failure/recovery, literal after unload; loaded=5 unloaded=5 peak=2", os.Getpid())
}

func incompatible(source []byte) map[string][]byte {
	s := string(source)
	return map[string][]byte{
		"signature":         []byte(strings.Replace(s, "pause func()) string", "pause func()) int", 1)),
		"dependency":        []byte(strings.Replace(s, `"fmt"`, `"fmt"; "time"`, 1)),
		"global":            []byte(s + "\nvar retained string\n"),
		"named-type":        []byte(s + "\ntype Changed struct { N int }\n"),
		"init":              []byte(s + "\nfunc init() {}\n"),
		"goroutine":         []byte(strings.Replace(s, "pause()", "go pause()", 1)),
		"escaping-callback": []byte(strings.Replace(s, "pause()", "fmt.Sprintf(\"%v\", pause)", 1)),
		"new-call":          []byte(strings.Replace(s, "pause()", "http.Get(\"https://example.com\")", 1)),
		"pointer-write":     []byte(strings.Replace(s, "pause()", "r.URL.Path = \"changed\"", 1)),
		"anonymous-type":    []byte(strings.Replace(s, "pause()", "x := struct{}{}; _ = x", 1)),
	}
}

func TestCompatibilityRejectsLifetimeAndABIChanges(t *testing.T) {
	source := fixture(t)
	if err := compatible(source); err != nil {
		t.Fatal(err)
	}
	for name, edit := range incompatible(source) {
		t.Run(name, func(t *testing.T) {
			if err := compatible(edit); !errors.Is(err, ErrRestart) {
				t.Fatalf("want restart, got %v", err)
			}
		})
	}
}

func TestRepeatedReloadWithConcurrentHTTPAndGC(t *testing.T) {
	r, err := newReloader(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	source := []byte(strings.ReplaceAll(string(fixture(t)), "original-literal", "stable-literal"))
	if _, err := r.Reload(source); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(r)
	defer server.Close()
	var wg sync.WaitGroup
	failures := make(chan error, 4)
	stop := make(chan struct{})
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 8
	defer transport.CloseIdleConnections()
	var requests int
	var countMu sync.Mutex
	// Stable output route independently checks each concurrent request; changed
	// computation is checked through /proof after every successful replacement.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client := &http.Client{Timeout: 10 * time.Second, Transport: transport}
			for {
				select {
				case <-stop:
					return
				default:
				}
				resp, err := client.Get(server.URL + "/literal")
				if err != nil {
					failures <- err
					return
				}
				body, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil || string(body) != "stable-literal" {
					failures <- fmt.Errorf("concurrent response %q err=%v", body, err)
					return
				}
				countMu.Lock()
				requests++
				countMu.Unlock()
				time.Sleep(time.Millisecond)
			}
		}()
	}
	// Each previous code/data mapping must actually disappear from the OS VM map.
	for i := 1; i <= 60; i++ {
		previousPages := mappedPages(r)
		assertMapped(t, previousPages, true)
		changed := edited(source, fmt.Sprintf("revision-%d", i), i+2)
		changed = []byte(strings.ReplaceAll(string(changed), fmt.Sprintf("revision-%d-literal", i), "stable-literal"))
		start := time.Now()
		times, err := r.Reload(changed)
		if err != nil {
			t.Fatal(err)
		}
		assertMapped(t, previousPages, false)
		assertMapped(t, mappedPages(r), true)
		want := fmt.Sprintf("revision-%d:%d:%d:/proof", i, i, 7*(i+2))
		mustGet(t, server.URL+"/proof?n=7", want)
		if i <= 3 || i == 60 {
			b, _ := json.Marshal(map[string]interface{}{"pid": os.Getpid(), "revision": i, "timing": times, "edit_to_http_ms": time.Since(start).Seconds() * 1000, "response": want})
			t.Log(string(b))
		}
		runtime.GC()
		if r.live != 1 || r.peak != 2 || r.loaded-r.unloaded != 1 {
			t.Fatal("unbounded module retention")
		}
	}
	close(stop)
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if requests < 100 {
		t.Fatalf("insufficient concurrent HTTP exercise: %d", requests)
	}
	lastPages := mappedPages(r)
	r.Close()
	assertMapped(t, lastPages, false)
	if r.loaded != 61 || r.unloaded != 61 || r.live != 0 {
		t.Fatalf("unbalanced modules: %d/%d/%d", r.loaded, r.unloaded, r.live)
	}
	t.Logf("pid=%d: 60 replacements, 60 exact changed HTTP responses, %d concurrent literal requests, forced GC each revision, loaded=61 unloaded=61 peak=2", os.Getpid(), requests)
}

func TestFrozenDependencyAndBuildInputs(t *testing.T) {
	r, err := newReloader(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	source := fixture(t)
	if _, err := r.Reload(source); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(r)
	defer server.Close()
	mustGet(t, server.URL+"/proof?n=7", "original:1:21:/proof")
	for _, key := range []string{"GOFLAGS", "GOEXPERIMENT", "GOARCH", "GOROOT"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "incompatible-poc-setting")
			if _, err := r.Reload(edited(source, "forbidden", 99)); !errors.Is(err, ErrRestart) {
				t.Fatalf("want explicit restart: %v", err)
			}
		})
	}
	mustGet(t, server.URL+"/proof?n=7", "original:2:21:/proof")
	// Only touch this experiment's private cloned source, never the installed
	// toolchain. Change a real imported dependency BODY, preserving its mtime.
	path := filepath.Join(runtime.GOROOT(), "src", "strconv", "number.go")
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resolved, "/ephemeral/reload-spike/goroot/src/") {
		t.Fatal("dependency edit requires isolated reload-spike GOROOT")
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(original), "func Atoi(s string) (int, error) {", "func Atoi(s string) (int, error) {\n return 99,nil", 1)
	if changed == string(original) {
		t.Fatal("pinned Atoi body missing")
	}
	t.Cleanup(func() {
		_ = os.WriteFile(path, original, info.Mode())
		_ = os.Chtimes(path, info.ModTime(), info.ModTime())
	})
	if err := os.WriteFile(path, []byte(changed), info.Mode()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reload(edited(source, "forbidden", 99)); !errors.Is(err, ErrRestart) {
		t.Fatalf("changed dependency body accepted: %v", err)
	}
	mustGet(t, server.URL+"/proof?n=7", "original:3:21:/proof")
	if err := os.WriteFile(path, original, info.Mode()); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reload(edited(source, "restored", 5)); err != nil {
		t.Fatal(err)
	}
	mustGet(t, server.URL+"/proof?n=7", "restored:4:35:/proof")
	extra := filepath.Join(filepath.Dir(path), "native_reload_poc_added.go")
	if _, err := os.Stat(extra); !os.IsNotExist(err) {
		t.Fatal("unexpected preexisting membership fixture")
	}
	if err := os.WriteFile(extra, []byte("package strconv\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(extra) })
	if _, err := r.Reload(source); !errors.Is(err, ErrRestart) {
		t.Fatalf("changed dependency membership accepted: %v", err)
	}
	if err := os.Remove(extra); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reload(source); err != nil {
		t.Fatal(err)
	}
	mustGet(t, server.URL+"/proof?n=7", "original:5:21:/proof")
	t.Log("actual strconv body edit (same mtime), dependency membership, compiler environment rejected; dependency and source reverts accepted with retained state")
}
