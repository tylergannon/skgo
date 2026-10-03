package dev

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	server, err := New(Config{
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
	})
	if err != nil {
		t.Fatal(err)
	}
	addr, err := server.Listen()
	if err != nil {
		t.Fatal(err)
	}
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
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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

	// The generator ran for every build and its own output started none.
	settled := s.gens.Load()
	time.Sleep(time.Second)
	if now := s.gens.Load(); now != settled {
		t.Fatalf("generated files retriggered the watcher: %d generations, then %d", settled, now)
	}
	if settled < 5 {
		t.Fatalf("the generator ran %d times for five source changes and the first build", settled)
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
