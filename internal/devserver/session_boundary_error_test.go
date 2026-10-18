//go:build skgo_dev_integration

package devserver

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRealSessionComponentBoundaryErrorHook(t *testing.T) {
	realSession(t, false, false, "boundary-error")
}

func setupBoundaryError(t *testing.T, write func(string, string)) {
	t.Helper()
	for _, name := range []string{"+page.svelte", "Failure.svelte"} {
		body, err := os.ReadFile(filepath.Join("../../example/web/src/routes/error/component-boundary", name))
		if err != nil {
			t.Fatal(err)
		}
		write("web/src/routes/error/component-boundary/"+name, string(body))
	}
}

func configureBoundaryErrorHook(t *testing.T, app string) {
	t.Helper()
	path := filepath.Join(app, "server.go")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Replace(string(body), `import("io/fs";`, `import("context";"io/fs";`, 1)
	source = strings.Replace(source, `loads,err:=skgo.NewLoads(manifest.LoadConfig(origin),generated.Loads()...);`, `loadCfg:=manifest.LoadConfig(origin);loadCfg.HandleError=func(ctx context.Context,caught skgo.CaughtError)map[string]any {
 if event:=skgo.EventFrom(ctx);event==nil || event.Request().URL.Path!="/error/component-boundary" {panic("boundary hook lost request event")}
 extra:=map[string]any{"supportId":"case-1121"};if caught.Kind=="unknown" {extra["message"]="Something went wrong on our end."};return extra
};loads,err:=skgo.NewLoads(loadCfg,generated.Loads()...);`, 1)
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
}

func exerciseBoundaryError(t *testing.T, origin string, log *sessionLog) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, tc := range []struct {
		kind                       string
		status                     int
		message, support, original string
	}{
		{"unknown", 500, "Something went wrong on our end.", "case-1121", "no original field"},
		{"app", 409, "literal-public-component-app-353", "case-1121", "literal-public-original-353"},
		{"framework", 404, "literal-public-framework-353", "case-1121", "no original field"},
		{"validation", 400, "Bad Request", "case-1121", "no original field"},
		{"handled", 418, "literal-already-handled-353", "no hook field", "literal-public-original-353"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			res, err := client.Get(origin + "/error/component-boundary?kind=" + tc.kind)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(res.Body)
			res.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != tc.status {
				t.Fatalf("status=%d body=%s\n%s", res.StatusCode, body, log.String())
			}
			for _, literal := range []string{"Component boundary policy", tc.message, tc.support, tc.original} {
				if !strings.Contains(string(body), literal) {
					t.Errorf("document lacks %q: %s\n%s", literal, body, log.String())
				}
			}
			for _, literal := range []string{"literal-private-component-cause-353", "literal-private-framework-cause-353", "literal-private-validation-cause-353"} {
				if strings.Contains(string(body), literal) {
					t.Errorf("private cause leaked: %s", body)
				}
			}
			if tc.kind == "handled" && strings.Contains(string(body), "case-1121") {
				t.Error("already handled error ran hook twice")
			}
		})
	}
	res, err := client.Get(origin + "/error/component-boundary?kind=redirect")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 307 || res.Header.Get("Location") != "/about" || len(body) != 0 {
		t.Fatalf("redirect=%d %v %s", res.StatusCode, res.Header, body)
	}
}
