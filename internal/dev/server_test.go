package dev

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// The application under supervision is an ordinary Go program, written here
// as literal source and edited while it runs. Nothing about a later version is
// compiled into an earlier one: each response below can only come from a
// binary built after the edit that wrote its literal.

const fixtureMod = "module fixture\n\ngo 1.27.1\n"

const fixtureMain = `package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"fixture/answer"
)

func main() {
	listen := flag.String("listen", "", "")
	flag.String("proxy", "", "")
	flag.String("origin", "", "")
	flag.Parse()
	mux := http.NewServeMux()
	register(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s pid=%d host=%s", answer.Text(), os.Getpid(), r.Host)
	})
	if err := http.ListenAndServe(*listen, mux); err != nil {
		panic(err)
	}
}
`

const fixtureRegister = `package main

import "net/http"

func register(*http.ServeMux) {}
`

func answerSource(literal string) string {
	return "package answer\n\nfunc Text() string { return " + strconv.Quote(literal) + " }\n"
}

type session struct {
	server *Server
	t      *testing.T
	root   string
	base   string
	cancel context.CancelFunc
	done   chan error
	log    *syncBuffer
	gens   atomic.Int32
	// While gate is set a generation blocks on it, after announcing itself.
	gate atomic.Pointer[gateState]
}

type gateState struct {
	entered chan struct{}
	release chan struct{}
}

func startSession(t *testing.T) *session {
	t.Helper()
	return startSessionPolling(t, 20*time.Millisecond)
}

func startSessionPolling(t *testing.T, poll time.Duration) *session {
	return startSessionConfigured(t, poll, nil)
}

func startSessionConfigured(t *testing.T, poll time.Duration, configure func(*Config)) *session {
	t.Helper()
	t.Setenv("GOWORK", "off")
	root := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", fixtureMod)
	write("cmd/main.go", fixtureMain)
	write("cmd/register.go", fixtureRegister)
	write("answer/answer.go", answerSource("revision one"))
	if err := os.MkdirAll(filepath.Join(root, "web", "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	s := &session{t: t, root: root, log: &syncBuffer{}, done: make(chan error, 1)}
	cfg := Config{
		Root: root, Web: filepath.Join(root, "web"), Out: filepath.Join(root, "generated"),
		Package: "./cmd", Listen: "127.0.0.1:0",
		Args:   func(listen string) []string { return []string{"-listen", listen} },
		Stdout: s.log, Stderr: s.log,
		Poll: poll, Quiet: 30 * time.Millisecond,
		Generate: func(context.Context) ([]byte, error) {
			// The generator's output is a Go file in the watched module. If
			// the watcher counted it as authored, every build would cause
			// another.
			s.gens.Add(1)
			if g := s.gate.Load(); g != nil {
				g.entered <- struct{}{}
				<-g.release
			}
			return nil, os.WriteFile(filepath.Join(root, "answer", "zz_gen.go"), []byte(fmt.Sprintf("// Code generated. DO NOT EDIT.\n\npackage answer\n\nconst Generation = %d\n", s.gens.Load())), 0o644)
		},
	}
	if configure != nil {
		configure(&cfg)
	}
	server, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	addr, err := server.Listen()
	if err != nil {
		t.Fatal(err)
	}
	s.server = server
	s.base = "http://" + addr.String()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() { s.done <- server.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-s.done:
		case <-time.After(20 * time.Second):
		}
	})
	return s
}

func (s *session) write(rel, content string) {
	s.t.Helper()
	path := filepath.Join(s.root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		s.t.Fatal(err)
	}
	// Publish one complete authored save. Truncating the watched file first can
	// expose an intermediate state for an entire debounce under CI scheduling.
	// A build that starts then legitimately needs another when the write ends.
	temporary, err := os.CreateTemp(filepath.Dir(path), ".skgo-source-*")
	if err != nil {
		s.t.Fatal(err)
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.WriteString(content); err != nil {
		temporary.Close()
		s.t.Fatal(err)
	}
	if err := temporary.Close(); err != nil {
		s.t.Fatal(err)
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		s.t.Fatal(err)
	}
}

type reply struct {
	status int
	body   string
	header http.Header
}

func (s *session) get(path string) reply {
	resp, err := http.Get(s.base + path)
	if err != nil {
		return reply{status: -1, body: err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return reply{status: resp.StatusCode, body: string(body), header: resp.Header}
}

// until polls a path until accept is satisfied and returns that reply.
func (s *session) until(path string, accept func(reply) bool, what string) reply {
	s.t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	var last reply
	for time.Now().Before(deadline) {
		last = s.get(path)
		if accept(last) {
			return last
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.t.Fatalf("%s: last answer %d %q\nsupervisor log:\n%s", what, last.status, last.body, s.log.String())
	return last
}

func pidOf(t *testing.T, body string) int {
	t.Helper()
	_, after, ok := strings.Cut(body, "pid=")
	if !ok {
		t.Fatalf("no pid in %q", body)
	}
	digits, _, _ := strings.Cut(after, " ")
	pid, err := strconv.Atoi(digits)
	if err != nil {
		t.Fatalf("pid in %q: %v", body, err)
	}
	return pid
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func waitDead(t *testing.T, pid int, what string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for alive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("%s: pid %d is still running", what, pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func startsWith(prefix string) func(reply) bool {
	return func(r reply) bool { return r.status == 200 && strings.HasPrefix(r.body, prefix) }
}

func TestDevReplacesTheApplicationAsAuthoredGoChanges(t *testing.T) {
	s := startSession(t)

	first := s.until("/", startsWith("revision one "), "initial build")
	if !strings.Contains(first.body, "host="+strings.TrimPrefix(s.base, "http://")) {
		t.Fatalf("the application did not see the public Host: %q", first.body)
	}
	firstPID := pidOf(t, first.body)

	// A body edit: the next answer is a different binary with the literal the
	// edit wrote, and the one it replaced is gone.
	s.write("answer/answer.go", answerSource("revision two"))
	second := s.until("/", startsWith("revision two "), "edited literal")
	if pid := pidOf(t, second.body); pid == firstPID {
		t.Fatalf("revision two was answered by the first process, pid %d", pid)
	}
	waitDead(t, firstPID, "the replaced application")

	// A compile error is reported with its file and message, and nothing
	// answers with revision two in its place.
	s.write("answer/answer.go", "package answer\n\nfunc Text() string { return 7 }\n")
	broken := s.until("/", func(r reply) bool { return r.status == http.StatusInternalServerError }, "compile error")
	if broken.header.Get("X-Skgo-Dev-Error") == "" {
		t.Fatalf("the failure is not marked as a build failure: %v", broken.header)
	}
	for _, want := range []string{"answer/answer.go:3", "cannot use 7"} {
		if !strings.Contains(broken.body, want) {
			t.Fatalf("diagnostic lacks %q:\n%s", want, broken.body)
		}
	}
	if strings.Contains(broken.body, "revision") {
		t.Fatalf("the failure page carries a stale answer: %q", broken.body)
	}
	secondPID := pidOf(t, second.body)
	waitDead(t, secondPID, "the application that was current when the error was introduced")

	// Correcting it recovers inside the same session.
	s.write("answer/answer.go", answerSource("revision three"))
	third := s.until("/", startsWith("revision three "), "recovery")
	if pidOf(t, third.body) == secondPID {
		t.Fatal("recovery was answered by the stopped process")
	}

	// A handler and route that did not exist when the session began. The
	// file is written now; no earlier binary can contain it.
	s.write("cmd/register.go", `package main

import (
	"fmt"
	"net/http"
)

func register(mux *http.ServeMux) {
	mux.HandleFunc("/added", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "New Go route revision one") })
}
`)
	s.until("/added", func(r reply) bool { return r.status == 200 && r.body == "New Go route revision one" }, "added route")
	s.write("cmd/register.go", fixtureRegister)
	s.until("/added", func(r reply) bool { return r.status == 200 && strings.HasPrefix(r.body, "revision three ") }, "removed route falls to the root handler")

	// Five complete source saves plus the initial build mean six generations.
	// Anchor the expectation in those literal edits, not in the observed counter.
	if got := s.gens.Load(); got != 6 {
		t.Fatalf("five source changes and initial build: got %d generations, want 6\nsupervisor log:\n%s", got, s.log.String())
	}
	time.Sleep(time.Second)
	if got := s.gens.Load(); got != 6 {
		t.Fatalf("generated files retriggered the watcher: got %d generations, want 6\nsupervisor log:\n%s", got, s.log.String())
	}

	// Shutdown stops what the session started.
	last := pidOf(t, s.get("/").body)
	s.cancel()
	select {
	case err := <-s.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the session did not stop")
	}
	waitDead(t, last, "the application at shutdown")
	if _, err := http.Get(s.base + "/"); err == nil {
		t.Fatal("the public address still answers after shutdown")
	}
}

func TestDevReportsAnApplicationThatExits(t *testing.T) {
	s := startSession(t)
	s.until("/", startsWith("revision one "), "initial build")

	s.write("cmd/register.go", `package main

import "net/http"

func register(mux *http.ServeMux) { panic("boom at startup") }
`)
	failed := s.until("/", func(r reply) bool { return r.status == http.StatusInternalServerError }, "startup failure")
	if !strings.Contains(failed.body, "boom at startup") {
		t.Fatalf("the failure does not say why the application stopped:\n%s", failed.body)
	}

	s.write("cmd/register.go", fixtureRegister)
	s.until("/", startsWith("revision one "), "recovery after a crash")
}

func TestDevHoldsRequestsWhileTheReplacementBuilds(t *testing.T) {
	s := startSession(t)
	s.until("/", startsWith("revision one "), "initial build")

	gate := &gateState{entered: make(chan struct{}, 1), release: make(chan struct{})}
	s.gate.Store(gate)
	s.write("answer/answer.go", answerSource("revision two"))
	select {
	case <-gate.entered:
	case <-time.After(30 * time.Second):
		t.Fatal("the edit did not start a build")
	}

	// The replaced application is still running, and must not answer.
	answered := make(chan reply, 1)
	go func() { answered <- s.get("/") }()
	select {
	case r := <-answered:
		t.Fatalf("a request was answered while the replacement was building: %d %q", r.status, r.body)
	case <-time.After(500 * time.Millisecond):
	}

	s.gate.Store(nil)
	close(gate.release)
	select {
	case r := <-answered:
		if r.status != 200 || !strings.HasPrefix(r.body, "revision two ") {
			t.Fatalf("the held request was answered %d %q", r.status, r.body)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("the held request was never answered")
	}
}

// Kit lists a route the moment its file exists, long before the next poll of
// the watcher. A request in that interval is not answered by the application
// that was built without the edit.
func TestDevHoldsARequestThatArrivesBeforeThePollSeesTheEdit(t *testing.T) {
	s := startSessionPolling(t, 3*time.Second)
	first := s.until("/", startsWith("revision one "), "initial build")
	firstPID := pidOf(t, first.body)

	s.write("answer/answer.go", answerSource("revision two"))
	got := s.get("/")
	if got.status != 200 || !strings.HasPrefix(got.body, "revision two ") {
		t.Fatalf("a request right after the edit was answered %d %q", got.status, got.body)
	}
	if pidOf(t, got.body) == firstPID {
		t.Fatalf("the replaced application, pid %d, answered after the edit", firstPID)
	}
}

func TestDevDrainsAnAssignedResponseBeforeRetiringItsApplication(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	gate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		fmt.Fprint(w, "released")
	}))
	defer gate.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	s := startSession(t)
	s.until("/", startsWith("revision one "), "initial build")
	s.write("cmd/register.go", `package main
import ("fmt"; "net/http"; "os"; "fixture/answer")
func register(mux *http.ServeMux) {
 mux.HandleFunc("/held", func(w http.ResponseWriter, r *http.Request) {
  _, err := http.Get(`+strconv.Quote(gate.URL)+`)
  if err != nil { panic(err) }
  fmt.Fprintf(w, "%s pid=%d held complete", answer.Text(), os.Getpid())
 })
}
`)
	old := s.until("/", startsWith("revision one "), "held handler build")
	// Wait until the authored handler is registered, then start its response.
	// The source fingerprint makes the next request wait for that build.
	result := make(chan reply, 1)
	go func() { result <- s.get("/held") }()
	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		t.Fatal("held handler did not start")
	}
	oldPID := pidOf(t, old.body)
	s.write("answer/answer.go", answerSource("revision two"))
	s.until("/", startsWith("revision two "), "replacement build")
	select {
	case response := <-result:
		t.Fatalf("held response ended before its gate opened: %d %q", response.status, response.body)
	case <-time.After(100 * time.Millisecond):
	}
	if !alive(oldPID) {
		t.Fatal("application was retired with an assigned response still open")
	}
	close(release)
	select {
	case response := <-result:
		if response.status != 200 || !strings.HasPrefix(response.body, "revision one ") || !strings.HasSuffix(response.body, "held complete") {
			t.Fatalf("assigned response was interrupted: %d %q", response.status, response.body)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("held response did not complete")
	}
	waitDead(t, oldPID, "the drained application")
}

// A save can be paused after truncation while the watcher starts building.
// Completing it during generation is another authored edit, not generator output.
func TestDevRebuildsForASaveCompletedDuringGeneration(t *testing.T) {
	s := startSession(t)
	s.until("/", startsWith("revision one "), "initial build")
	gate := &gateState{entered: make(chan struct{}, 1), release: make(chan struct{})}
	s.gate.Store(gate)
	f, err := os.OpenFile(filepath.Join(s.root, "answer", "answer.go"), os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	defer func() {
		select {
		case <-gate.release:
		default:
			close(gate.release)
		}
	}()
	select {
	case <-gate.entered:
	case <-time.After(30 * time.Second):
		t.Fatal("intermediate save did not start generation")
	}
	if _, err := f.WriteString(answerSource("revision two")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	s.gate.Store(nil)
	close(gate.release)
	s.until("/", startsWith("revision two "), "completed save")
	deadline := time.After(30 * time.Second)
	for s.gens.Load() != 3 {
		select {
		case <-deadline:
			t.Fatalf("completed authored save did not schedule generation 3; got %d\n%s", s.gens.Load(), s.log.String())
		case <-time.After(20 * time.Millisecond):
		}
	}
	s.until("/", startsWith("revision two "), "follow-up build")
	time.Sleep(time.Second)
	if got := s.gens.Load(); got != 3 {
		t.Fatalf("completed save: got %d generations, want 3\n%s", got, s.log.String())
	}
}

func TestDevCanTransferAnExclusiveApplicationResource(t *testing.T) {
	reloads := make(chan string, 4)
	vite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/__skgo_dev/supervisor" || r.Header.Get("X-Skgo-Dev-Token") != "exclusive-fixture" {
			t.Errorf("unexpected Vite request %s", r.URL)
			w.WriteHeader(403)
			return
		}
		var body struct{ Diagnostic string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		reloads <- body.Diagnostic
		w.WriteHeader(204)
	}))
	defer vite.Close()
	exclusive, err := freeAddr()
	if err != nil {
		t.Fatal(err)
	}
	s := startSessionConfigured(t, 20*time.Millisecond, func(cfg *Config) {
		cfg.StopBeforeStart = true
		cfg.Vite, _ = url.Parse(vite.URL)
		if err := os.MkdirAll(filepath.Join(cfg.Web, ".svelte-kit"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cfg.Web, ".svelte-kit/skgo-dev-supervisor.json"), []byte(`{"token":"exclusive-fixture","base":"/"}`), 0600); err != nil {
			t.Fatal(err)
		}
		cfg.Env = []string{"EXCLUSIVE_ADDR=" + exclusive}
		source := strings.Replace(fixtureMain, `"net/http"`, `"net/http"; "net"`, 1)
		source = strings.Replace(source, "func main() {", `func main() {
 lease,err:=net.Listen("tcp",os.Getenv("EXCLUSIVE_ADDR"));if err!=nil{panic(err)};defer lease.Close()
`, 1)
		if err := os.WriteFile(filepath.Join(cfg.Root, "cmd/main.go"), []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	})
	first := s.get("/")
	if first.status != 200 || !strings.Contains(first.body, "revision one") {
		t.Fatalf("first: %+v", first)
	}
	select {
	case got := <-reloads:
		t.Fatalf("initial readiness sent browser reload %q", got)
	default:
	}
	s.write("answer/answer.go", answerSource("exclusive replacement"))
	next := s.get("/")
	if next.status != 200 || !strings.Contains(next.body, "exclusive replacement") {
		t.Fatalf("replacement did not take exclusive resource: %+v\n%s", next, s.log.String())
	}
	select {
	case got := <-reloads:
		if got != "" {
			t.Fatalf("replacement sent failure %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("exclusive replacement did not reload connected browser")
	}
}

func TestDevExclusiveReplacementClosesEventStreams(t *testing.T) {
	s := startSessionConfigured(t, 20*time.Millisecond, func(cfg *Config) {
		cfg.StopBeforeStart = true
		if err := os.WriteFile(filepath.Join(cfg.Root, "cmd/register.go"), []byte(`package main
import("fmt";"net/http";"fixture/answer")
func register(mux *http.ServeMux) {
 mux.HandleFunc("/events",func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Content-Type","text/event-stream; charset=utf-8")
  fmt.Fprintf(w,"data: %s\n\n",answer.Text());w.(http.Flusher).Flush()
  <-r.Context().Done()
 })
}
`), 0644); err != nil {
			t.Fatal(err)
		}
	})
	response, err := http.Get(s.base + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	prefix := make([]byte, len("data: revision one\n\n"))
	if _, err := io.ReadFull(response.Body, prefix); err != nil {
		t.Fatal(err)
	}
	if string(prefix) != "data: revision one\n\n" {
		t.Fatalf("stream=%q", prefix)
	}
	ended := make(chan struct{})
	go func() { io.Copy(io.Discard, response.Body); close(ended) }()
	s.write("answer/answer.go", answerSource("after stream replacement"))
	replaced := make(chan reply, 1)
	go func() { replaced <- s.get("/") }()
	select {
	case got := <-replaced:
		if got.status != 200 || !strings.HasPrefix(got.body, "after stream replacement ") {
			t.Fatalf("replacement=%+v\n%s", got, s.log.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("open event stream held up replacement\n%s", s.log.String())
	}
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("old event stream survived replacement")
	}
}

func TestDevWatchesCapturedReplacementSourcesAndNewEmbedMatches(t *testing.T) {
	dependency := t.TempDir()
	put := func(path, text string) {
		t.Helper()
		path = filepath.Join(dependency, path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	put("go.mod", "module fixturedep\n\ngo 1.27.1\n")
	put("answer.go", `package answer
import("embed";"strconv")
//go:embed texts/*.txt
var assets embed.FS
func Text() string { data,_:=assets.ReadFile("texts/value.txt");files,_:=assets.ReadDir("texts");return string(data)+" entries="+strconv.Itoa(len(files)) }
`)
	put("texts/value.txt", "dependency one")
	s := startSessionConfigured(t, 20*time.Millisecond, func(cfg *Config) {
		mod := fixtureMod + "\nrequire fixturedep v0.0.0\nreplace fixturedep => " + strconv.Quote(dependency) + "\n"
		if err := os.WriteFile(filepath.Join(cfg.Root, "go.mod"), []byte(mod), 0644); err != nil {
			t.Fatal(err)
		}
		source := strings.Replace(fixtureMain, `"fixture/answer"`, `answer "fixturedep"`, 1)
		if err := os.WriteFile(filepath.Join(cfg.Root, "cmd/main.go"), []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
		cfg.WatchInputs = func() WatchInputs {
			return WatchInputs{Roots: []string{dependency}, Files: []string{filepath.Join(dependency, "go.mod")}, EmbedPatterns: []string{filepath.Join(dependency, "texts/*.txt")}}
		}
	})
	first := s.get("/")
	if first.status != 200 || !strings.Contains(first.body, "dependency one entries=1") {
		t.Fatalf("first=%+v\n%s", first, s.log.String())
	}
	dependencySource := filepath.Join(dependency, "answer.go")
	original, err := os.ReadFile(dependencySource)
	if err != nil {
		t.Fatal(err)
	}
	put("answer.go", strings.Replace(string(original), `return string(data)+`, `return "authored dependency " + string(data)+`, 1))
	body := s.get("/")
	if body.status != 200 || !strings.Contains(body.body, "authored dependency dependency one entries=1") {
		t.Fatalf("external Go body edit=%+v\n%s", body, s.log.String())
	}
	file := filepath.Join(dependency, "texts/value.txt")
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	put("texts/value.txt", "dependency two")
	if err := os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	edited := s.get("/")
	if edited.status != 200 || !strings.Contains(edited.body, "dependency two entries=1") {
		t.Fatalf("preserved timestamp edit=%+v\n%s", edited, s.log.String())
	}
	put("texts/new.txt", "new embedded member")
	added := s.get("/")
	if added.status != 200 || !strings.Contains(added.body, "dependency two entries=2") {
		t.Fatalf("new embed match=%+v\n%s", added, s.log.String())
	}
}
