package newapp

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Exercise the shipped scaffold's entry point as a real process. The handler
// supplies held HTTP requests; lifecycle code is the generated cmd/main.go.
func TestGeneratedApplicationShutdown(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := writeGoFiles(project{Dir: dir, Module: "example.com/shutdown", BindingsImport: "example.com/shutdown/internal/skgo", GoVersion: "1.27.2", SkgoVersion: "v0.0.0", SkgoReplace: repo, App: "shutdown", Origin: "http://127.0.0.1:8080", OriginHostPort: "127.0.0.1:8080"}); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dir, map[string]string{"server.go": shutdownHandler})
	mainPath := filepath.Join(dir, "cmd/main.go")
	source, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	// A developer's application cleanup belongs in run, beside initialization.
	// Add only that deferred observer; leave generated serving code untouched.
	body := strings.Replace(string(source), "func run() error {", `func run() error {
 defer app.Cleanup()
`, 1)
	if err := os.WriteFile(mainPath, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "shutdown-app")
	build := exec.CommandContext(t.Context(), "go", "build", "-mod=mod", "-o", binary, "./cmd")
	build.Dir = dir
	build.Env = append(os.Environ(), "GOWORK=off")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build generated application: %v\n%s", err, output)
	}
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	t.Cleanup(client.CloseIdleConnections)

	for _, signal := range []os.Signal{syscall.SIGTERM, os.Interrupt} {
		t.Run(signal.String(), func(t *testing.T) {
			process := startShutdownApplication(t, binary, "")
			response, err := client.Get(process.origin + "/held")
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			prefix := make([]byte, len("literal-start-361|"))
			if _, err := io.ReadFull(response.Body, prefix); err != nil || string(prefix) != "literal-start-361|" || response.StatusCode != 200 {
				t.Fatalf("held request did not start: status=%d prefix=%q err=%v", response.StatusCode, prefix, err)
			}
			if err := process.cmd.Process.Signal(signal); err != nil {
				t.Fatal(err)
			}
			process.waitForListenerClose(t)
			select {
			case <-process.done:
				t.Fatalf("generated application exited before held request finished: %v\n%s", process.err, process.log.String())
			default:
			}
			if _, err := os.Stat(process.cleanup); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("application cleanup ran before request drained: %v", err)
			}
			if err := os.WriteFile(process.release, []byte("release-held-request"), 0600); err != nil {
				t.Fatal(err)
			}
			rest, err := io.ReadAll(response.Body)
			if err != nil || string(rest) != "literal-finished-361" {
				t.Fatalf("signal interrupted held request: body=%q err=%v", rest, err)
			}
			if err := process.wait(t, 3*time.Second); err != nil {
				t.Fatalf("drained application did not exit cleanly: %v\n%s", err, process.log.String())
			}
			if cleanup := readFile(t, process.cleanup); cleanup != "literal-cleanup-361:literal-handler-finished-361" {
				t.Fatalf("cleanup did not run after request completion: %q", cleanup)
			}
		})
	}

	t.Run("stream timeout", func(t *testing.T) {
		process := startShutdownApplication(t, binary, "")
		response, err := client.Get(process.origin + "/stream")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		prefix := make([]byte, len("literal-stream-361\n"))
		if _, err := io.ReadFull(response.Body, prefix); err != nil || string(prefix) != "literal-stream-361\n" || response.StatusCode != 200 {
			t.Fatalf("long-lived stream did not start: status=%d prefix=%q err=%v", response.StatusCode, prefix, err)
		}
		started := time.Now()
		if err := process.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		process.waitForListenerClose(t)
		_, streamErr := io.ReadAll(response.Body)
		if streamErr == nil {
			t.Fatal("unfinished stream falsely completed successfully")
		}
		if err := process.wait(t, 2*time.Second); err == nil {
			t.Fatal("shutdown deadline did not report failure")
		}
		elapsed := time.Since(started)
		if elapsed < 9*time.Second || elapsed > 13*time.Second {
			t.Fatalf("stream shutdown took %s, want the scaffold's ten-second bound", elapsed)
		}
		if !strings.Contains(process.log.String(), "shutdown: context deadline exceeded") {
			t.Fatalf("shutdown timeout diagnostic missing:\n%s", process.log.String())
		}
		if cleanup := readFile(t, process.cleanup); cleanup != "literal-cleanup-361:literal-stream-canceled-361" {
			t.Fatalf("timed-out application cleanup did not run: %q", cleanup)
		}
	})

	t.Run("listen failure", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		process := startShutdownApplication(t, binary, listener.Addr().String())
		if err := process.wait(t, 3*time.Second); err == nil {
			t.Fatal("generated application reported success after failing to listen")
		}
		if !strings.Contains(process.log.String(), "address already in use") {
			t.Fatalf("listen failure diagnostic missing:\n%s", process.log.String())
		}
		if cleanup := readFile(t, process.cleanup); cleanup != "literal-cleanup-361:" {
			t.Fatalf("serve failure bypassed application cleanup: %q", cleanup)
		}
	})
}

type shutdownApplication struct {
	cmd                      *exec.Cmd
	log                      bytes.Buffer
	done                     chan struct{}
	err                      error
	origin, cleanup, release string
}

func startShutdownApplication(t *testing.T, binary, occupied string) *shutdownApplication {
	t.Helper()
	address := occupied
	if address == "" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		address = listener.Addr().String()
		listener.Close()
	}
	dir := t.TempDir()
	p := &shutdownApplication{done: make(chan struct{}), origin: "http://" + address, cleanup: filepath.Join(dir, "cleanup"), release: filepath.Join(dir, "release")}
	p.cmd = exec.CommandContext(t.Context(), binary, "-listen", address)
	p.cmd.Env = append(os.Environ(), "CLEANUP_FILE="+p.cleanup, "RELEASE_FILE="+p.release, "FINISHED_FILE="+filepath.Join(dir, "finished"))
	p.cmd.Stdout, p.cmd.Stderr = &p.log, &p.log
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { p.err = p.cmd.Wait(); close(p.done) }()
	t.Cleanup(func() {
		p.cmd.Process.Kill()
		p.wait(t, 3*time.Second)
	})
	if occupied == "" {
		client := &http.Client{Timeout: time.Second}
		defer client.CloseIdleConnections()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			res, err := client.Get(p.origin + "/ready")
			if err == nil {
				body, _ := io.ReadAll(res.Body)
				res.Body.Close()
				if res.StatusCode == 200 && string(body) == "literal-ready-361" {
					return p
				}
			}
			select {
			case <-p.done:
				t.Fatalf("generated application exited during startup: %v\n%s", p.err, p.log.String())
			default:
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("generated application never became ready")
	}
	return p
}

func (p *shutdownApplication) wait(t *testing.T, timeout time.Duration) error {
	t.Helper()
	select {
	case <-p.done:
		return p.err
	case <-time.After(timeout):
		t.Fatal("generated application failed to exit within deadline")
		return nil
	}
}

func (p *shutdownApplication) waitForListenerClose(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", strings.TrimPrefix(p.origin, "http://"), 50*time.Millisecond)
		if err != nil {
			return
		}
		connection.Close()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("generated server kept accepting connections after signal")
}

const shutdownHandler = `package app
import("fmt";"io";"io/fs";"net/http";"os";"time")
var streamStarted=make(chan struct{})
var streamCanceled=make(chan struct{})
func Cleanup(){
 finished,_:=os.ReadFile(os.Getenv("FINISHED_FILE"))
 marker:="literal-cleanup-361:"+string(finished)
 select{case <-streamStarted:
  select{case <-streamCanceled:marker+="literal-stream-canceled-361";case <-time.After(time.Second):marker+="literal-stream-still-active-361"}
 default:}
 if err:=os.WriteFile(os.Getenv("CLEANUP_FILE"),[]byte(marker),0600);err!=nil{panic(err)}
}
func NewHandler(_ fs.FS,_ string)(http.Handler,error){
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  switch r.URL.Path {
  case "/ready":io.WriteString(w,"literal-ready-361")
  case "/held":
   io.WriteString(w,"literal-start-361|");w.(http.Flusher).Flush()
   for {if _,err:=os.Stat(os.Getenv("RELEASE_FILE"));err==nil{break};select{case <-r.Context().Done():return;case <-time.After(5*time.Millisecond):}}
   if err:=os.WriteFile(os.Getenv("FINISHED_FILE"),[]byte("literal-handler-finished-361"),0600);err!=nil{panic(err)}
   fmt.Fprint(w,"literal-finished-361")
  case "/stream":close(streamStarted);io.WriteString(w,"literal-stream-361\n");w.(http.Flusher).Flush();<-r.Context().Done();close(streamCanceled)
  default:http.NotFound(w,r)
  }
 }),nil
}
`
