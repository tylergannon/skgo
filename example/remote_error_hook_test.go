package example_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/tylergannon/skgo"
	generated "github.com/tylergannon/skgo/example/internal/skgo"
)

func TestProductionRemoteErrorsUseConfiguredApplicationHook(t *testing.T) {
	handler := newProdHandler(t)
	for _, test := range []struct {
		name, payload, message string
		status                 int
	}{
		{"readSensor", "", "The sensor is being calibrated", 409},
		{"getDependent", devaluePayload(`["literal-private-remote-input-354"]`), "Something went wrong on our end.", 500},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := "/_app/remote/" + remoteID(t, test.name)
			if test.payload != "" {
				path += "?payload=" + test.payload
			}
			rec := get(t, handler, path)
			var response struct {
				Type  string `json:"type"`
				Error struct {
					Status    int    `json:"status"`
					Message   string `json:"message"`
					SupportID string `json:"supportId"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || response.Type != "error" || response.Error.Status != test.status || response.Error.Message != test.message || response.Error.SupportID != "case-1121" {
				t.Fatalf("production remote skipped configured public error policy: %d %s", rec.Code, rec.Body.String())
			}
			if rec.Header().Get("Cache-Control") != "private, no-store" || strings.Contains(rec.Body.String(), "dependent query did not receive its input") || strings.Contains(rec.Body.String(), "literal-private-remote-input-354") {
				t.Fatalf("production error leaked private cause or changed caching: %v %s", rec.Header(), rec.Body.String())
			}
		})
	}
}

func TestProductionDocumentRemoteErrorUsesConfiguredApplicationHook(t *testing.T) {
	rec := get(t, newProdHandler(t), "/error/boundary")
	if rec.Code != 409 || !strings.Contains(rec.Body.String(), `<h1 data-testid="title">Sensor</h1>`) || !strings.Contains(rec.Body.String(), `data-testid="sensor-failed">The sensor is being calibrated</p>`) {
		t.Fatalf("production error boundary did not render: %d %s", rec.Code, rec.Body.String())
	}
	entries := documentRemoteEntries(t, rec.Body.String())
	entry := entries[remoteID(t, "readSensor")+"/"].(map[string]any)
	public, ok := entry["e"].(map[string]any)
	if !ok || public["status"] != float64(409) || public["message"] != "The sensor is being calibrated" || public["supportId"] != "case-1121" {
		t.Fatalf("collected remote error skipped configured policy: %#v", entry)
	}
}

// Keep the real built components and Go application boundary. Only the remote
// body and hook are supplied by the fixture, to make single/batch failures and
// hook invocation counts independently observable without rebuilding a page.
func TestProductionDocumentRemoteFailuresAreHandledOnce(t *testing.T) {
	for _, batch := range []bool{false, true} {
		for _, outcome := range []string{"returned", "panic", "known"} {
			name := "single/" + outcome
			if batch {
				name = "batch/" + outcome
			}
			t.Run(name, func(t *testing.T) {
				calls := 0
				hook := func(ctx context.Context, caught skgo.CaughtError) map[string]any {
					calls++
					if skgo.EventFrom(ctx) == nil {
						t.Error("SSR remote hook lost its request event")
					}
					if outcome == "known" {
						if caught.Kind != "app" || caught.Status != 409 {
							t.Errorf("known remote failure classification: %+v", caught)
						}
						return map[string]any{"supportId": "literal-collected-hook-354"}
					}
					if caught.Kind != "unknown" || caught.Err == nil || !strings.Contains(caught.Err.Error(), "literal-private-collected-cause-354") {
						t.Errorf("unexpected remote failure lost its private cause: %+v", caught)
					}
					return map[string]any{"message": "literal-collected-public-354", "supportId": "literal-collected-hook-354"}
				}
				fail := func() error {
					if outcome == "panic" {
						panic("literal-private-collected-cause-354")
					}
					if outcome == "known" {
						return skgo.Errorf(409, "literal-collected-known-354")
					}
					return errors.New("literal-private-collected-cause-354")
				}
				remotes := generated.Remotes()
				function, path, title, failed := "readSensor", "/error/boundary", "Sensor", "sensor-failed"
				if batch {
					function, path, title, failed = "getQuotes", "/batch", "Batch", "quotes-failed"
				}
				var keys []string
				for i, fn := range remotes {
					if fn.Name() != function {
						continue
					}
					spec := skgo.RemoteSpec{Module: fn.Module(), Name: fn.Name(), Kind: fn.Kind(), Call: func(context.Context, skgo.Call) (any, error) { return nil, fail() }, Batch: func(context.Context, []skgo.Call) ([]any, error) { return nil, fail() }}
					remotes[i] = skgo.NewRemote(spec)
					if batch {
						for _, symbol := range []string{"SKGO", "GOJA", "KITX", "SVLT"} {
							keys = append(keys, fn.ID()+"/"+devaluePayload(`["`+symbol+`"]`))
						}
					} else {
						keys = []string{fn.ID() + "/"}
					}
				}
				if len(keys) == 0 {
					t.Fatalf("missing generated remote %s", function)
				}
				handler := remoteErrorDocumentHandler(t, remotes, hook)
				rec := get(t, handler, path)
				status, message := 500, "literal-collected-public-354"
				if outcome == "known" {
					status, message = 409, "literal-collected-known-354"
				}
				if rec.Code != status || !strings.Contains(rec.Body.String(), `<h1 data-testid="title">`+title+`</h1>`) || !strings.Contains(rec.Body.String(), `data-testid="`+failed+`">`+message+`</p>`) {
					t.Fatalf("remote failure did not reach production boundary: %d %s", rec.Code, rec.Body.String())
				}
				if calls != 1 || strings.Contains(rec.Body.String(), "literal-private-collected-cause-354") {
					t.Fatalf("remote policy calls=%d, want exactly one and no private details: %s", calls, rec.Body.String())
				}
				entries := documentRemoteEntries(t, rec.Body.String())
				for _, key := range keys {
					entry, ok := entries[key].(map[string]any)
					if !ok {
						t.Fatalf("missing collected remote key %s in %#v", key, entries)
					}
					public, ok := entry["e"].(map[string]any)
					if !ok || public["status"] != float64(status) || public["message"] != message || public["supportId"] != "literal-collected-hook-354" {
						t.Fatalf("collected remote key %s lost public error fields: %#v", key, entry)
					}
				}
			})
		}
	}
}

func documentRemoteEntries(t *testing.T, document string) map[string]any {
	t.Helper()
	assignment := regexp.MustCompile(`__sveltekit_[A-Za-z0-9_]+\.data = ([^\n]+);`).FindStringSubmatch(document)
	if len(assignment) != 2 {
		t.Fatal("production document has no remote hydration data")
	}
	value, err := goja.New().RunString("JSON.stringify(" + assignment[1] + ")")
	if err != nil {
		t.Fatal(err)
	}
	var data struct{ Q map[string]any }
	if err := json.Unmarshal([]byte(value.String()), &data); err != nil || data.Q == nil {
		t.Fatalf("remote hydration data has no query entries: %s: %v", value, err)
	}
	return data.Q
}

func remoteErrorDocumentHandler(t *testing.T, functions []*skgo.Remote, hook skgo.HandleError) http.Handler {
	t.Helper()
	dist := prodDist(t)
	manifest := buildManifest(t)
	remoteConfig, loadConfig := manifest.RemoteConfig(prodOrigin), manifest.LoadConfig(prodOrigin)
	remoteConfig.HandleError, loadConfig.HandleError = hook, hook
	remoteConfig.OnPanic = func(string, any, []byte) {}
	remoteConfig.Transport, loadConfig.Transport = generated.Transport(), generated.Transport()
	loadConfig.Matchers = generated.Matchers()
	remotes, err := skgo.NewRemotes(remoteConfig, functions...)
	if err != nil {
		t.Fatal(err)
	}
	loads, err := skgo.NewLoads(loadConfig, generated.Loads()...)
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := skgo.NewSSR(dist, manifest, loads, remotes, skgo.SSROptions{Runtimes: 1})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := skgo.NewStaticHandler(dist, skgo.WithSSR(renderer))
	if err != nil {
		t.Fatal(err)
	}
	config := manifest.HandleConfig()
	config.Loads, config.Origin, config.Matchers, config.HandleError = loads, prodOrigin, generated.Matchers(), hook
	return generated.RequestBoundary(config, loads.Intercept(remotes.Intercept(pages)))
}
