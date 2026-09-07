//go:build quickjs

// A comparison point only. quickjs-go is cgo, which is the opposite of what
// this project wants ("one binary"), so it is behind a build tag:
//
//	go test -tags quickjs ./internal/ssrspike/ -run TestQuickJS -v
package ssrspike

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	quickjs "github.com/buke/quickjs-go"
)

func TestQuickJSComparison(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	dir := spikeDir(t)
	src, err := os.ReadFile(filepath.Join(dir, "dist", "ssr.js"))
	if err != nil {
		t.Fatal(err)
	}
	_, f := load(t)

	newCtx := func() (*quickjs.Runtime, *quickjs.Context) {
		rt := quickjs.NewRuntime()
		ctx := rt.NewContext()
		ctx.Globals().Set("__skgo_remote", ctx.Function(func(c *quickjs.Context, _ *quickjs.Value, args []*quickjs.Value) *quickjs.Value {
			key := args[0].ToString() + "/" + args[1].ToString()
			out, _ := json.Marshal(f.Remotes[key])
			return c.String(string(out))
		}))
		return rt, ctx
	}

	start := time.Now()
	rt, ctx := newCtx()
	defer rt.Close()
	defer ctx.Close()
	res := ctx.Eval(string(src))
	if res.IsException() {
		t.Fatalf("quickjs could not evaluate the bundle: %v", ctx.Exception())
	}
	res.Free()
	ctx.Loop()
	t.Logf("quickjs fresh Runtime + evaluate bundle: %v", time.Since(start).Round(time.Microsecond))

	render := func(name string) string {
		req, _ := json.Marshal(json.RawMessage(f.Requests[name]))
		v := ctx.Eval("__skgo_render(" + string(req2str(req)) + ")")
		if v.IsException() {
			t.Fatalf("quickjs render %s: %v", name, ctx.Exception())
		}
		defer v.Free()
		ctx.Loop()
		body := v.Get("body")
		defer body.Free()
		return body.ToString()
	}

	for _, name := range []string{"probe", "account", "error"} {
		body := render(name)
		if len(body) == 0 {
			t.Errorf("quickjs rendered %s to nothing", name)
			continue
		}
		if name == "probe" && !strings.Contains(body, `<p data-testid="site-name">Anvil and Ampersand</p>`) {
			t.Errorf("quickjs did not answer the remote function in process:\n%s", body)
		}
		const n = 50
		start := time.Now()
		for range n {
			render(name)
		}
		t.Logf("quickjs warm render %-8s %v  (%d bytes)", name, (time.Since(start) / n).Round(time.Microsecond), len(body))
	}
}

// req2str produces a JS string literal for the request JSON.
func req2str(req []byte) []byte {
	out, _ := json.Marshal(string(req))
	return out
}
