package devrender_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Only the disposable copy gets backend selection and passive event observation.
// Neither config is edited after startup: Vite tracks imported config files too.
func configureWatcherFixture() error {
	if err := os.Rename(filepath.Join(webRoot, "vite.config.ts"), filepath.Join(webRoot, "fixture-base.ts")); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(webRoot, "vite.config.ts"), []byte(watcherFixture), 0644)
}

const watcherFixture = `
import base from './fixture-base';
import { mergeConfig } from 'vite-plus';
import { resolve, isAbsolute } from 'node:path';
import { writeFileSync } from 'node:fs';
const backend = process.env.SKGO_TEST_WATCHER_BACKEND ?? 'fswatch';
if (!['fswatch', 'native', 'polling'].includes(backend)) throw new Error('unknown test watcher backend');
const watch = backend === 'native' ? {} : { useFsEvents: false, usePolling: backend === 'polling' };
export default mergeConfig(base, {
 server: { watch },
 plugins: [{
  name: 'skgo-watcher-fixture',
  configureServer(server) {
   const trace = [], errors = [];
   const file = resolve(server.config.root, 'src/lib/watcher-restore.ts');
   const record = (kind, path) => { if (path === file) trace.push({kind, ms: performance.now(), file: path}); };
   server.watcher.on('change', path => record('change', resolve(path)));
   server.watcher.on('raw', (_, path, details) => {
    const watched = details?.watchedPath;
    const full = isAbsolute(path) ? resolve(path) : watched === file ? file : resolve(watched ?? server.config.root, path);
    record('raw', full);
   });
   const send = server.ws.send;
   server.ws.send = function (...args) {
    if (args[0]?.type === 'error') errors.push(args[0].err.message);
    return send.apply(this, args);
   };
   server.middlewares.use('/__watcher_fixture', (req, res) => {
    if (new URL(req.url, 'http://fixture').searchParams.has('restore')) {
     record('restore-write', file);
     writeFileSync(file, 'export const probe = 1;\n');
    }
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify({options: server.watcher.options, trace, errors}));
   });
  }
 }]
});
`

type watcherObservation struct {
	Options json.RawMessage
	Trace   []struct {
		Kind string
		MS   float64
		File string
	}
	Errors []string
}

func fetchWatcher(t *testing.T, path string) string {
	t.Helper()
	resp, err := http.Get(devServer + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, resp.StatusCode, b)
	}
	return string(b)
}

func observeWatcher(t *testing.T, query string) watcherObservation {
	t.Helper()
	var out watcherObservation
	if err := json.Unmarshal([]byte(fetchWatcher(t, "/__watcher_fixture"+query)), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSettledRestoreReachesViteModule(t *testing.T) {
	const file = "src/lib/watcher-restore.ts"
	const original = "export const probe = 1;\n"
	const edited = "export const probe = 2;\n"
	path := filepath.Join(webRoot, filepath.FromSlash(file))
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	if body := fetchWatcher(t, "/"+file); !strings.Contains(body, "probe = 1") {
		t.Fatalf("initial module: %s", body)
	}
	settleChanges(t, file, 0)
	cursor, _ := changeLog(t, 0)
	start := len(observeWatcher(t, "").Trace)
	if err := os.WriteFile(path, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(10 * time.Second)
	for namedChanges(t, file, cursor) < 1 {
		if time.Now().After(until) {
			t.Fatal("first edit never delivered")
		}
		time.Sleep(time.Millisecond)
	}
	if body := fetchWatcher(t, "/"+file); !strings.Contains(body, "probe = 2") || strings.Contains(body, "probe = 1") {
		t.Fatalf("edited module: %s", body)
	}
	// Same Node clock records the actual restore write and public raw witness.
	// No Go rendering or quiet wait belongs in this race-sensitive interval.
	observeWatcher(t, "?restore")
	var final string
	until = time.Now().Add(10 * time.Second)
	for {
		final = fetchWatcher(t, "/"+file)
		if strings.Contains(final, "probe = 1") && !strings.Contains(final, "probe = 2") && namedChanges(t, file, cursor) == 2 {
			break
		}
		if time.Now().After(until) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	observation := observeWatcher(t, "")
	trace := observation.Trace[start:]
	delivered, restored, witness := -1.0, -1.0, -1.0
	for _, event := range trace {
		if event.Kind == "change" && delivered < 0 {
			delivered = event.MS
		}
		if event.Kind == "restore-write" {
			restored = event.MS
		}
		if event.Kind == "raw" && restored >= 0 && witness < 0 {
			witness = event.MS
		}
	}
	t.Logf("watcher.options=%s trace=%+v restore_raw_after_first_delivery_ms=%.3f in_window=%t", observation.Options, trace, witness-delivered, restored >= 0 && witness >= restored && delivered >= 0 && witness-delivered >= 0 && witness-delivered < 50)
	disk, err := os.ReadFile(path)
	if err != nil || string(disk) != original {
		t.Fatalf("restored disk=%q error=%v", disk, err)
	}
	if n := namedChanges(t, file, cursor); n != 2 {
		t.Errorf("delivered edits=%d, want two separately observed edits", n)
	}
	if !strings.Contains(final, "probe = 1") || strings.Contains(final, "probe = 2") {
		t.Errorf("restored module serves stale bytes: %s", final)
	}
}

func awaitDocument(t *testing.T, before, after string) {
	t.Helper()
	until := time.Now().Add(20 * time.Second)
	var body string
	for {
		response := get("/go-dev")
		body = response.Body.String()
		if response.Code == 200 && strings.Contains(body, after) && !strings.Contains(body, before) {
			return
		}
		if time.Now().After(until) {
			t.Fatalf("document did not replace %q with %q: %s", before, after, body)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestSettledRestoreReachesGoDocument(t *testing.T) {
	path := filepath.Join(webRoot, "src/routes/go-dev/+page.svelte")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Independent literal text in the existing Go-rendered route component.
	const before = "Watcher document original"
	const after = "Watcher document restored"
	original := append(append([]byte{}, raw...), []byte("\n<p>"+before+"</p>\n")...)
	edited := bytes.Replace(original, []byte(before), []byte(after), 1)
	defer os.WriteFile(path, raw, 0644)
	for _, stage := range []struct {
		bytes           []byte
		absent, present string
	}{{original, after, before}, {edited, before, after}, {original, after, before}} {
		if err := os.WriteFile(path, stage.bytes, 0644); err != nil {
			t.Fatal(err)
		}
		awaitDocument(t, stage.absent, stage.present)
	}
	if disk, err := os.ReadFile(path); err != nil || !bytes.Equal(disk, original) {
		t.Fatalf("restored component bytes mismatch: %v", err)
	}
}

func TestPartialTemplateSavesRecover(t *testing.T) {
	path := filepath.Join(webRoot, "src/app.html")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer os.WriteFile(path, raw, 0644)
	observation := observeWatcher(t, "")
	var opts struct {
		AwaitWriteFinish struct {
			StabilityThreshold int
			PollInterval       int
		}
	}
	if err := json.Unmarshal(observation.Options, &opts); err != nil {
		t.Fatal(err)
	}
	threshold := time.Duration(opts.AwaitWriteFinish.StabilityThreshold) * time.Millisecond
	poll := time.Duration(opts.AwaitWriteFinish.PollInterval) * time.Millisecond
	if threshold <= 0 || poll <= 0 {
		t.Fatalf("partial-save qualification needs configured stability: %s", observation.Options)
	}
	for i, mode := range []string{"coalesced", "delivered", "absent"} {
		t.Run(mode, func(t *testing.T) {
			before, after := fmt.Sprintf("Watcher template before %d", i), fmt.Sprintf("Watcher template after %d", i)
			complete := func(marker string) []byte {
				return bytes.Replace(raw, []byte("</head>"), []byte("<meta name=\"watcher-template\" content=\""+marker+"\" /></head>"), 1)
			}
			if err := os.WriteFile(path, complete(before), 0644); err != nil {
				t.Fatal(err)
			}
			awaitDocument(t, after, before)
			errorsBefore := len(observeWatcher(t, "").Errors)
			if mode == "absent" {
				err = os.Remove(path)
			} else {
				err = os.WriteFile(path, []byte("<html>unfinished save"), 0644)
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "coalesced" {
				time.Sleep(threshold / 4)
			} else {
				code := "app_template_tag_missing"
				if mode == "absent" {
					code = "app_template_missing"
				}
				until := time.Now().Add(10*time.Second + threshold + poll)
				for {
					errors := observeWatcher(t, "").Errors[errorsBefore:]
					found := false
					for _, message := range errors {
						if strings.HasPrefix(message, code+"\n") && strings.Contains(message, "src/app.html") {
							found = true
						}
					}
					if found {
						break
					}
					if time.Now().After(until) {
						t.Fatalf("held partial save never reported Kit %s: %v", code, errors)
					}
					time.Sleep(poll)
				}
			}
			if err := os.WriteFile(path, complete(after), 0644); err != nil {
				t.Fatal(err)
			}
			awaitDocument(t, before, after)
			if mode == "coalesced" && len(observeWatcher(t, "").Errors) != errorsBefore {
				t.Fatalf("save completed inside quiet window emitted errors: %+v", observeWatcher(t, "").Errors[errorsBefore:])
			}
		})
	}
}
