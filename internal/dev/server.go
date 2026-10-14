// Package dev is the development supervisor for an application whose server is
// written in Go.
//
// Kit's dev server keeps its route graph current while it runs: it watches the
// route tree, rebuilds its manifest on a debounce and rewrites its generated
// files (exports/vite/dev/index.js update_manifest, core/sync/sync.js create).
// Everything it re-reads is JavaScript. A Go server body is compiled code, so
// the equivalent here is a loop that watches the developer's Go source,
// regenerates the bindings Kit compiles, rebuilds the application and puts the
// new process behind the same public address.
//
// The supervisor owns the public listener. The application is a child on a
// private port, replaced rather than patched, and requests that arrive while a
// replacement is being built wait for it instead of being answered by the
// version being replaced. A build that fails is reported, with the file and
// message, as the response to every request until the source is corrected; it
// is never answered with the last version that did compile.
package dev

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config describes one development session.
type Config struct {
	// Root is the Go module root. Builds run here and every authored .go file
	// beneath it is watched.
	Root string
	// Web is the Vite root, and Out the generated bindings directory. Out is
	// the generator's output and is never watched.
	Web, Out string
	// Package is the main package of the application, relative to Root.
	Package string
	// Listen is the public address the supervisor serves.
	Listen string
	// Args are the arguments the application is started with, given the
	// private address it must listen on.
	Args func(listen string) []string
	// Vite is the running Vite dev server. A WebSocket upgrade goes straight to
	// it, so the HMR socket stays connected while the application behind it is
	// replaced. Nil routes everything to the application.
	Vite *url.URL
	// ViteCommand starts an owned Vite process in Web. It is the complete
	// argv, including host and port matching Vite. Nil attaches to Vite instead.
	ViteCommand []string
	// ViteEnv and Env append environment overrides for Vite and the app.
	ViteEnv, Env []string
	// BuildFlags are passed to go build before the package argument.
	BuildFlags []string
	// StopBeforeStart is for applications holding exclusive resources such as
	// a catalog lock. Compilation completes first, then the previous process
	// drains and exits before its replacement starts.
	StopBeforeStart bool
	// ExtraWatch contains additional authored files or directories, relative to
	// Root unless absolute. Their contents trigger the ordinary generation loop.
	ExtraWatch []string
	// RestartFiles are inputs compiled into a retained Generate callback. Editing
	// them produces a restart-required diagnostic instead of using stale code.
	RestartFiles []string
	// WatchInputs returns discovery already owned by Generate, after it runs.
	// The supervisor never invokes the Go tool to rediscover those inputs.
	WatchInputs func() WatchInputs
	// Generate regenerates what Kit and Go compile from authored source. Its
	// output is shown with the diagnostic if it fails. Nil skips generation.
	Generate func(ctx context.Context) ([]byte, error)

	Stdout, Stderr io.Writer
	// Poll is how often authored source is checked, and Quiet how long it must
	// hold still before a build starts. Kit debounces its own watcher by the
	// same 100ms.
	Poll, Quiet time.Duration
	// StartTimeout bounds how long a new application may take to listen.
	StartTimeout time.Duration
}

// Server supervises the application.
type Server struct {
	cfg     Config
	inputMu sync.RWMutex
	in      inputs
	vite    http.Handler

	tmp      string
	listener net.Listener

	mu         sync.Mutex
	settled    chan struct{} // closed while no build is in progress
	building   bool
	current    *generation
	hasSettled bool

	// built is the fingerprint the last build started from.
	built string

	builds             int
	children           sync.WaitGroup
	stopping           chan struct{}
	reloads            chan string
	restartFingerprint string
}

// generation is what answers requests between two source changes: a running
// application, or the reason there is none.
type generation struct {
	proxy      http.Handler
	proc       *Process
	diagnostic string
	pid        int
	requests   sync.WaitGroup
	streamMu   sync.Mutex
	streams    map[*streamRequest]struct{}
	retiring   bool
}

type streamRequest struct{ cancel context.CancelFunc }
type streamRequestKey struct{}

func (g *generation) trackStream(request *streamRequest) {
	g.streamMu.Lock()
	defer g.streamMu.Unlock()
	if g.retiring {
		request.cancel()
		return
	}
	if g.streams == nil {
		g.streams = make(map[*streamRequest]struct{})
	}
	g.streams[request] = struct{}{}
}

func (g *generation) closeStreams() {
	g.streamMu.Lock()
	defer g.streamMu.Unlock()
	g.retiring = true
	for request := range g.streams {
		request.cancel()
	}
}

// New prepares a session. Nothing is started until Run.
func New(cfg Config) (*Server, error) {
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return nil, err
	}
	cfg.Root = root
	if cfg.Web, err = filepath.Abs(cfg.Web); err != nil {
		return nil, err
	}
	if cfg.Out != "" {
		if cfg.Out, err = filepath.Abs(cfg.Out); err != nil {
			return nil, err
		}
	}
	if len(cfg.ViteCommand) > 0 && cfg.Vite == nil {
		return nil, errors.New("skgo dev: owned Vite command requires its URL")
	}
	if cfg.Args == nil {
		return nil, errors.New("skgo dev: no application arguments")
	}
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}
	if cfg.Poll == 0 {
		cfg.Poll = 150 * time.Millisecond
	}
	if cfg.Quiet == 0 {
		cfg.Quiet = 100 * time.Millisecond
	}
	if cfg.StartTimeout == 0 {
		cfg.StartTimeout = 90 * time.Second
	}
	s := &Server{cfg: cfg, in: inputs{root: cfg.Root, web: cfg.Web, out: cfg.Out, digests: &fileDigests{}, extra: append(append([]string(nil), cfg.ExtraWatch...), cfg.RestartFiles...)}, building: true, settled: make(chan struct{}), stopping: make(chan struct{}), reloads: make(chan string, 1)}
	if cfg.Vite != nil {
		s.vite = &httputil.ReverseProxy{
			Rewrite: func(p *httputil.ProxyRequest) {
				p.SetURL(cfg.Vite)
				// Vite checks the inbound Host against its allowed hosts; the
				// browser's loopback host is accepted without configuration.
				p.Out.Host = p.In.Host
				p.SetXForwarded()
			},
		}
	}
	return s, nil
}

// Listen binds the public address. It is separate from Run so a caller that
// asked for port 0 can learn the address before the first build finishes.
func (s *Server) Listen() (net.Addr, error) {
	l, err := net.Listen("tcp", s.cfg.Listen)
	if err != nil {
		return nil, err
	}
	s.listener = l
	return l.Addr(), nil
}

// Run serves until ctx is cancelled, then stops the application it started.
func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var err error
	s.restartFingerprint, err = fingerprintPaths(s.cfg.Root, s.cfg.RestartFiles)
	if err != nil {
		return err
	}
	reloadDone := make(chan struct{})
	go func() { defer close(reloadDone); s.reloadLoop(ctx) }()
	defer func() { cancel(); <-reloadDone }()
	if s.listener != nil {
		defer s.listener.Close()
	}
	viteFailure := make(chan error, 1)
	if len(s.cfg.ViteCommand) > 0 {
		cmd := exec.Command(s.cfg.ViteCommand[0], s.cfg.ViteCommand[1:]...)
		cmd.Dir = s.cfg.Web
		cmd.Env = append(os.Environ(), s.cfg.ViteEnv...)
		proc, err := StartProcess(cmd, s.cfg.Stdout, s.cfg.Stderr)
		if err != nil {
			return fmt.Errorf("skgo dev: starting Vite: %w", err)
		}
		s.logf("started vite pid %d on %s", proc.PID(), s.cfg.Vite.Host)
		defer func() { cancel(); proc.Stop(5 * time.Second) }()
		go func() {
			select {
			case <-ctx.Done():
			case <-proc.Done():
				if ctx.Err() == nil {
					err := proc.ExitError()
					if err == nil {
						err = errors.New("process exited successfully")
					}
					viteFailure <- fmt.Errorf("skgo dev: vite exited unexpectedly: %w", err)
					cancel()
				}
			}
		}()
	}
	if s.listener == nil {
		if _, err := s.Listen(); err != nil {
			return err
		}
	}
	tmp, err := os.MkdirTemp("", "skgo-dev-")
	if err != nil {
		return err
	}
	s.tmp = tmp
	defer os.RemoveAll(tmp)

	srv := &http.Server{Handler: s}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(s.listener) }()

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		s.loop(ctx)
	}()

	var runErr error
	select {
	case <-ctx.Done():
	case err := <-served:
		if !errors.Is(err, http.ErrServerClosed) {
			runErr = err
		}
	}
	cancel()
	close(s.stopping)
	<-loopDone
	shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
	_ = srv.Close()
	s.mu.Lock()
	current := s.current
	s.current = nil
	s.mu.Unlock()
	if current != nil && current.proc != nil {
		s.logf("stopping application pid %d", current.pid)
		current.proc.Stop(3 * time.Second)
	}
	s.children.Wait()
	select {
	case err := <-viteFailure:
		return err
	default:
		return runErr
	}
}

func (s *Server) logf(format string, args ...any) {
	fmt.Fprintf(s.cfg.Stderr, "skgo dev: "+format+"\n", args...)
}

// loop is the whole of the watcher: build once, then build again whenever the
// authored inputs differ from what the last build saw.
func (s *Server) loop(ctx context.Context) {
	first := true
	for ctx.Err() == nil {
		if !first {
			s.mu.Lock()
			built := s.built
			s.mu.Unlock()
			if !s.awaitChange(ctx, built) {
				return
			}
			s.logf("source changed")
		}
		first = false
		s.setBuilding()
		// Taken before generating: an edit that lands during the build
		// differs from this and starts the next one.
		fingerprint, _ := s.fingerprint()
		s.mu.Lock()
		s.built = fingerprint
		s.mu.Unlock()
		next := s.build(ctx)
		if ctx.Err() != nil {
			if next.proc != nil {
				next.proc.Stop(3 * time.Second)
			}
			return
		}
		s.settle(next)
	}
}

// awaitChange blocks until the inputs differ from built and have then stopped
// changing for Quiet. It reports false if ctx ended first.
func (s *Server) awaitChange(ctx context.Context, built string) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(s.cfg.Poll):
		}
		now, err := s.fingerprint()
		if err != nil || now == built {
			continue
		}
		for {
			select {
			case <-ctx.Done():
				return false
			case <-time.After(s.cfg.Quiet):
			}
			again, err := s.fingerprint()
			if err != nil {
				continue
			}
			if again == now {
				return true
			}
			now = again
		}
	}
}

func (s *Server) setBuilding() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.building {
		s.building = true
		s.settled = make(chan struct{})
	}
}

// settle makes next the generation that answers requests, releases the
// requests that were waiting for it, and stops the one it replaces.
func (s *Server) settle(next *generation) {
	s.mu.Lock()
	previous := s.current
	broadcast := s.hasSettled || next.diagnostic != ""
	s.hasSettled = true
	s.current = next
	s.building = false
	close(s.settled)
	s.mu.Unlock()
	// Vite buffers full-reload when nobody is connected. Broadcasting initial
	// success would reload the very first visitor unnecessarily on connection.
	if broadcast {
		s.reload(next.diagnostic)
	}
	if previous != nil && previous.proc != nil {
		s.children.Add(1)
		go func() {
			defer s.children.Done()
			s.retire(context.Background(), previous)
		}()
	}
}

// retire preserves requests already assigned to the old process. Stop also
// waits for the process to exit, which releases exclusive application resources.
func (s *Server) retire(ctx context.Context, previous *generation) {
	// EventSource responses intentionally never finish. Reconnect them to the
	// replacement instead of spending the finite-response drain budget on them.
	previous.closeStreams()
	drained := make(chan struct{})
	go func() { previous.requests.Wait(); close(drained) }()
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case <-drained:
	case <-ctx.Done():
	case <-s.stopping:
	case <-timer.C:
		s.logf("application pid %d did not drain within 30s", previous.pid)
	}
	s.logf("stopping application pid %d", previous.pid)
	previous.proc.Stop(3 * time.Second)
}

func (s *Server) failed(report string) *generation {
	report = strings.TrimSpace(authoredPaths(report, s.cfg.Root, s.cfg.Web))
	s.logf("the application did not build:\n%s", report)
	return &generation{diagnostic: report}
}

// build regenerates, compiles and starts one application.
func (s *Server) build(ctx context.Context) *generation {
	current, err := fingerprintPaths(s.cfg.Root, s.cfg.RestartFiles)
	if err != nil {
		return s.failed(err.Error())
	}
	if current != s.restartFingerprint {
		return s.failed("Generator implementation changed. Restart the development command to load its new implementation.")
	}
	if s.cfg.Generate != nil {
		for {
			started := time.Now()
			s.logf("generating")
			if out, err := s.cfg.Generate(ctx); err != nil {
				if ctx.Err() != nil {
					return &generation{}
				}
				return s.failed(fmt.Sprintf("skgo generate failed: %v\n%s", err, out))
			}
			s.logf("generated in %s", time.Since(started).Round(time.Millisecond))
			if !s.updateInputs() {
				break
			}
			// A new source selection must be observed before generation. Repeat
			// against the expanded watch set so an edit during capture cannot be
			// acknowledged as built without entering the generator. Stable warm
			// selections do not enter this branch.
			fingerprint, err := s.fingerprint()
			if err != nil {
				return s.failed(err.Error())
			}
			s.mu.Lock()
			s.built = fingerprint
			s.mu.Unlock()
			if ctx.Err() != nil {
				return &generation{}
			}
		}
	}
	s.builds++
	binary := filepath.Join(s.tmp, "app-"+strconv.Itoa(s.builds))
	args := append([]string{"build", "-o", binary}, s.cfg.BuildFlags...)
	args = append(args, s.cfg.Package)
	compile := exec.CommandContext(ctx, "go", args...)
	compile.Dir = s.cfg.Root
	s.logf("building %s", s.cfg.Package)
	compileStarted := time.Now()
	if out, err := compile.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return &generation{}
		}
		return s.failed(fmt.Sprintf("go build %s failed: %v\n%s", s.cfg.Package, err, out))
	}
	s.logf("compiled in %s", time.Since(compileStarted).Round(time.Millisecond))
	startTime := time.Now()
	if s.cfg.StopBeforeStart {
		s.mu.Lock()
		previous := s.current
		s.current = nil
		s.mu.Unlock()
		if previous != nil && previous.proc != nil {
			s.retire(ctx, previous)
		}
	}
	if ctx.Err() != nil {
		return &generation{}
	}
	next, err := s.start(ctx, binary)
	if err != nil {
		if ctx.Err() != nil {
			return &generation{}
		}
		return s.failed(err.Error())
	}
	s.logf("replacement ready in %s", time.Since(startTime).Round(time.Millisecond))
	return next
}

func (s *Server) start(ctx context.Context, binary string) (*generation, error) {
	addr, err := freeAddr()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(binary, s.cfg.Args(addr)...)
	cmd.Dir = s.cfg.Root
	cmd.Env = append(os.Environ(), s.cfg.Env...)
	proc, err := StartProcess(cmd, s.cfg.Stdout, s.cfg.Stderr)
	if err != nil {
		return nil, err
	}
	s.logf("started application pid %d on %s", proc.PID(), addr)

	deadline := time.After(s.cfg.StartTimeout)
	for {
		if conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond); err == nil {
			conn.Close()
			break
		}
		select {
		case <-proc.Done():
			return nil, fmt.Errorf("the application exited before it listened: %v\n%s", proc.ExitError(), proc.tail.String())
		case <-ctx.Done():
			proc.Stop(3 * time.Second)
			return nil, ctx.Err()
		case <-deadline:
			proc.Stop(3 * time.Second)
			return nil, fmt.Errorf("the application did not listen within %s\n%s", s.cfg.StartTimeout, proc.tail.String())
		case <-time.After(50 * time.Millisecond):
		}
	}

	target := &url.URL{Scheme: "http", Host: addr}
	next := &generation{proc: proc, pid: proc.PID()}
	next.proxy = &httputil.ReverseProxy{
		Rewrite: func(p *httputil.ProxyRequest) {
			p.SetURL(target)
			p.Out.Host = p.In.Host
			p.SetXForwarded()
		},
		// Pages stream: a flushed chunk must reach the browser as it is
		// written.
		FlushInterval: -1,
		ModifyResponse: func(response *http.Response) error {
			kind, _, _ := strings.Cut(response.Header.Get("Content-Type"), ";")
			if strings.EqualFold(strings.TrimSpace(kind), "text/event-stream") {
				if request, ok := response.Request.Context().Value(streamRequestKey{}).(*streamRequest); ok {
					next.trackStream(request)
				}
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "skgo dev: the application at "+addr+" did not answer: "+err.Error(), http.StatusBadGateway)
		},
	}
	go s.watchExit(next)
	return next, nil
}

// watchExit reports an application that stops on its own. Until the source
// changes again it is the application's absence, not its last answer, that
// requests see.
func (s *Server) watchExit(g *generation) {
	<-g.proc.Done()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != g {
		return
	}
	report := fmt.Sprintf("the application (pid %d) exited: %v\n%s", g.pid, g.proc.ExitError(), g.proc.tail.String())
	s.logf("%s", report)
	s.current = &generation{diagnostic: report}
	s.reload(report)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.vite != nil && (isUpgrade(r) || s.isViteClient(r.URL.Path)) {
		s.vite.ServeHTTP(w, r)
		return
	}
	s.noticeEdit()
	g := s.awaitSettled(r.Context())
	switch {
	case g == nil:
		http.Error(w, "skgo dev: the application is still building", http.StatusServiceUnavailable)
	case g.diagnostic != "":
		writeDiagnostic(w, g.diagnostic, s.vite != nil, s.viteClientPath()+"client")
	default:
		defer g.requests.Done()
		ctx, cancel := context.WithCancel(r.Context())
		stream := &streamRequest{cancel: cancel}
		defer func() {
			cancel()
			g.streamMu.Lock()
			delete(g.streams, stream)
			g.streamMu.Unlock()
		}()
		g.proxy.ServeHTTP(w, r.WithContext(context.WithValue(ctx, streamRequestKey{}, stream)))
	}
}

// noticeEdit holds a request that arrives after an edit and before the next
// poll has seen it. The application that is still running was built from the
// source as it was, and answering from it would hand a visitor the old
// behaviour, or a route kit already lists with none of its Go behind it.
func (s *Server) noticeEdit() {
	s.mu.Lock()
	if s.building {
		s.mu.Unlock()
		return
	}
	built := s.built
	s.mu.Unlock()
	fingerprint, err := s.fingerprint()
	if err != nil || fingerprint == built {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// The loop is idle with this fingerprint, so it will see the difference
	// and build; a loop that already moved on has nothing left to wait for.
	if !s.building && s.built == built {
		s.building = true
		s.settled = make(chan struct{})
	}
}

func (s *Server) awaitSettled(ctx context.Context) *generation {
	for {
		s.mu.Lock()
		if !s.building {
			g := s.current
			if g != nil && g.proxy != nil {
				g.requests.Add(1)
			}
			s.mu.Unlock()
			return g
		}
		settled := s.settled
		s.mu.Unlock()
		select {
		case <-settled:
		case <-ctx.Done():
			return nil
		}
	}
}

func writeDiagnostic(w http.ResponseWriter, report string, vite bool, clientPath string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Skgo-Dev-Error", "build")
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "<!doctype html><title>skgo dev: build failed</title><h1>The Go application did not build</h1><pre data-testid=\"skgo-dev-diagnostic\">%s</pre><p>Fix the source; the server rebuilds by itself.</p>\n", html.EscapeString(report))
	if vite {
		fmt.Fprintf(w, "<script type=\"module\" src=\"%s\"></script>\n", html.EscapeString(clientPath))
	}
}

func isUpgrade(r *http.Request) bool {
	for _, token := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
			return r.Header.Get("Upgrade") != ""
		}
	}
	return false
}

func freeAddr() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	return l.Addr().String(), nil
}
