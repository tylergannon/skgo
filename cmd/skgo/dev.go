package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/tylergannon/skgo/internal/dev"
)

const devUsage = `usage: skgo dev [flags]

Starts Vite and the Go application once and keeps both current. Go source,
signatures and routes are regenerated and the application is rebuilt and
replaced behind the same address whenever they change. A build that fails is
reported, with its file and message, to the terminal and to every request until
the source is corrected.

`

// devCommand supervises one development session. It returns the process exit
// code.
func devCommand(args []string) int {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, devUsage)
		fs.PrintDefaults()
	}
	root := fs.String("root", ".", "Go module root; its Go source is watched and built from here")
	web := fs.String("web", "web", "the vite root, relative to --root")
	out := fs.String("out", "", "generated bindings directory, relative to --root; detected when omitted")
	pkg := fs.String("cmd", "./cmd", "the application's main package, relative to --root")
	listen := fs.String("listen", "127.0.0.1:8080", "public address to serve")
	origin := fs.String("origin", "", "public browser origin; defaults to http://--listen")
	viteCmd := fs.String("vite", "node_modules/.bin/vp dev", "command that starts the Vite dev server in --web; --host, --port and --strictPort are appended")
	vitePort := fs.Int("vite-port", 5173, "port Vite listens on; 0 picks a free one")
	viteURL := fs.String("vite-url", "", "attach to a Vite dev server that is already running instead of starting one")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return 2
	}

	rootDir, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	webDir := *web
	if !filepath.IsAbs(webDir) {
		webDir = filepath.Join(rootDir, webDir)
	}
	outDir := *out
	if outDir == "" {
		outDir = "generated"
		if fi, err := os.Stat(filepath.Join(rootDir, "internal", "skgo")); err == nil && fi.IsDir() {
			outDir = filepath.Join("internal", "skgo")
		}
	}
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(rootDir, outDir)
	}
	if _, err := os.Stat(filepath.Join(rootDir, "go.mod")); err != nil {
		fmt.Fprintf(os.Stderr, "skgo dev: %s is not a Go module root: %v\n", rootDir, err)
		return 1
	}
	if *origin == "" {
		*origin = "http://" + *listen
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var vite *url.URL
	var viteProc *dev.Process
	viteFailure := make(chan error, 1)
	if *viteURL != "" {
		if vite, err = url.Parse(*viteURL); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	} else {
		port := *vitePort
		if port == 0 {
			if port, err = freePort(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
		}
		argv := strings.Fields(*viteCmd)
		if len(argv) == 0 {
			fmt.Fprintln(os.Stderr, "skgo dev: --vite is empty")
			return 2
		}
		argv = append(argv, "--host", "127.0.0.1", "--port", strconv.Itoa(port), "--strictPort")
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = webDir
		cmd.Env = append(os.Environ(), "ORIGIN="+*origin)
		if viteProc, err = dev.StartProcess(cmd, os.Stdout, os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "skgo dev: starting vite: %v\n", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "skgo dev: started vite pid %d on 127.0.0.1:%d\n", viteProc.PID(), port)
		vite = &url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}
		go func() {
			<-viteProc.Done()
			if ctx.Err() == nil {
				failure := viteProc.ExitError()
				if failure == nil {
					failure = fmt.Errorf("process exited successfully")
				}
				fmt.Fprintf(os.Stderr, "skgo dev: vite exited unexpectedly: %v\n", failure)
				viteFailure <- failure
				cancel()
			}
		}()
	}

	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	server, err := dev.New(dev.Config{
		Root: rootDir, Web: webDir, Out: outDir, Package: *pkg, Listen: *listen,
		Vite: vite,
		Args: func(private string) []string {
			return []string{"-listen", private, "-proxy", vite.String(), "-origin", *origin}
		},
		Generate: func(ctx context.Context) ([]byte, error) {
			// The same invocation `go generate` makes: from the bindings
			// package, so every path the generator reports is the one a
			// developer sees.
			cmd := exec.CommandContext(ctx, self, "generate", "--web", webDir, "--out", ".", "--quiet")
			cmd.Dir = outDir
			var output bytes.Buffer
			cmd.Stdout, cmd.Stderr = &output, &output
			err := cmd.Run()
			return output.Bytes(), err
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	addr, err := server.Listen()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "skgo dev: serving http://%s (origin %s)\n", addr, *origin)

	code := 0
	if err := server.Run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	select {
	case <-viteFailure:
		code = 1
	default:
	}
	if viteProc != nil {
		fmt.Fprintf(os.Stderr, "skgo dev: stopping vite pid %d\n", viteProc.PID())
		viteProc.Stop(5 * time.Second)
	}
	return code
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
