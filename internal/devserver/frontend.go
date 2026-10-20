package devserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tylergannon/skgo/internal/gen"
)

type frontendReady struct {
	Socket   string   `json:"socket"`
	Instance string   `json:"instance"`
	Tooling  []string `json:"tooling"`
}
type frontendSnapshot struct {
	RemotePrefix string             `json:"remotePrefix"`
	Dist         string             `json:"dist"`
	Metadata     *gen.RouteMetadata `json:"metadata"`
	Revision     string             `json:"revision"`
}
type frontend struct {
	*child
	ready     frontendReady
	client    *http.Client
	readyFile string
}

func startFrontend(ctx context.Context, o Options, web, work, origin, secret string) (*frontend, error) {
	if e := os.MkdirAll(work, 0700); e != nil {
		return nil, e
	}
	readyFile := filepath.Join(work, "vite-ready.json")
	_ = os.Remove(readyFile)
	_ = os.Remove(filepath.Join(work, "vite.sock"))
	env := append(os.Environ(), "ORIGIN="+origin, "SKGO_DEV_ORIGIN="+origin, "SKGO_DEV_DIR="+work, "SKGO_DEV_READY="+readyFile, "SKGO_DEV_GO_SOCKET="+filepath.Join(work, "go.sock"), "SKGO_DEV_VITE_SOCKET="+filepath.Join(work, "vite.sock"), "SKGO_DEV_SERVICE_SECRET="+secret, "SKGO_DEV_LIFELINE=stdin")
	vp := o.VitePlus
	if vp == "" {
		vp = "vp"
	}
	process, e := startChild(ctx, web, vp, []string{"exec", "node", "--input-type=module", "-e", `await import(new URL('./skgo-adapter/dev-host.js',import.meta.resolve('@skgo/sveltekit-adapter')));`}, env, o.Output)
	if e != nil {
		return nil, e
	}
	fmt.Fprintf(o.Output, "skgo dev: Vite child pid %d\n", process.cmd.Process.Pid)
	f := &frontend{child: process, readyFile: readyFile, client: &http.Client{Timeout: 2 * time.Minute, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(work, "vite.sock"))
	}}}}
	timer := time.NewTimer(45 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			f.close()
			return nil, ctx.Err()
		case <-process.done:
			return nil, fmt.Errorf("Vite exited before adapter readiness: %v", process.err)
		case <-timer.C:
			f.close()
			return nil, fmt.Errorf("Vite did not announce skgo development readiness; check that the app uses this version of @skgo/sveltekit-adapter")
		case <-tick.C:
			raw, e := os.ReadFile(readyFile)
			if e != nil {
				continue
			}
			if e = json.Unmarshal(raw, &f.ready); e != nil {
				continue
			}
			if f.ready.Socket == filepath.Join(work, "vite.sock") {
				// A readiness file alone is not readiness: exercise the listener.
				probe, cancel := context.WithTimeout(ctx, time.Second)
				err := f.request(probe, "state", nil, nil)
				cancel()
				if err == nil {
					return f, nil
				}
			}
		}
	}
}

// refresh follows Vite's own in-process restarts, which need not exit vp.
func (f *frontend) refresh() (bool, error) {
	raw, err := os.ReadFile(f.readyFile)
	if err != nil {
		return false, err
	}
	var ready frontendReady
	if err = json.Unmarshal(raw, &ready); err != nil {
		return false, err
	}
	if ready.Socket != f.ready.Socket || !filepath.IsAbs(ready.Socket) {
		return false, fmt.Errorf("invalid Vite readiness socket %q", ready.Socket)
	}
	changed := ready.Instance != f.ready.Instance
	if changed {
		f.ready = ready
	}
	return changed, nil
}

func (f *frontend) request(ctx context.Context, path string, body any, result any) error {
	var reader io.Reader
	method := "GET"
	if body != nil {
		raw, e := json.Marshal(body)
		if e != nil {
			return e
		}
		reader = bytes.NewReader(raw)
		method = "POST"
	}
	req, e := http.NewRequestWithContext(ctx, method, "http://localhost/__skgo/dev/"+path, reader)
	if e != nil {
		return e
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, e := f.client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		return fmt.Errorf("Vite %s: HTTP%d %s", path, res.StatusCode, raw)
	}
	if result != nil {
		return json.NewDecoder(res.Body).Decode(result)
	}
	_, e = io.Copy(io.Discard, res.Body)
	return e
}
func (f *frontend) snapshot(ctx context.Context) (frontendSnapshot, error) {
	var s frontendSnapshot
	e := f.request(ctx, "snapshot", nil, &s)

	return s, e
}
func (f *frontend) notify(ctx context.Context, revision gen.Revision, message string, reload bool) error {
	return f.request(ctx, "published", map[string]any{"revision": revision, "error": message, "reload": reload}, nil)
}
