package plugininstall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/buildinfo"
	"github.com/tylergannon/skgo/internal/pluginbuild"
	"github.com/tylergannon/skgo/internal/pluginstore"
)

func TestPublishFailureKeepsPriorSelection(t *testing.T) {
	dir := t.TempDir()
	slot := filepath.Join(dir, "module.so")
	ref := pluginstore.Reference(slot)
	os.WriteFile(slot, []byte("working prior binary"), 0755)
	os.WriteFile(ref, []byte("prior source"), 0644)
	if err := publish(filepath.Join(dir, "missing.so"), slot, pluginstore.Source{Module: "example.com/new", Version: "v1.0.0"}); err == nil {
		t.Fatal("missing candidate accepted")
	}
	for name, want := range map[string]string{slot: "working prior binary", ref: "prior source"} {
		if b, e := os.ReadFile(name); e != nil || string(b) != want {
			t.Fatalf("%s changed: %q %v", name, b, e)
		}
	}
	candidate := filepath.Join(dir, "candidate.so")
	os.WriteFile(candidate, []byte("new binary"), 0755)
	if err := publish(candidate, slot, pluginstore.Source{Module: "example.com/new", Version: "v1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(slot); string(b) != "new binary" {
		t.Fatalf("candidate not published: %q", b)
	}
	if b, _ := os.ReadFile(ref); string(b) != `{"module":"example.com/new","version":"v1.0.0"}` {
		t.Fatalf("source reference: %q", b)
	}
}
func TestHostLockSerializesAndCancellationReleasesWait(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".install.lock")
	unlock, err := lock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	second, err := lock(ctx, path)
	if err == nil {
		second()
		unlock()
		t.Fatal("two publishers held the same host lock")
	}
	unlock()
	third, err := lock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	third()
}

// The compiler and executable probe have their own boundary tests. Exercise
// Install's real orchestration and filesystem publication without building
// another host and four plugins for these decisions.
func TestInstallPublishesOnlyAfterBuildAndPeerQualification(t *testing.T) {
	for _, stage := range []string{"success", "build failure", "probe failure", "canceled"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			write := func(path, body string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0644); err != nil {
					t.Fatal(err)
				}
			}
			read := func(path string) string {
				t.Helper()
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				return string(body)
			}
			const hostBytes = "literal installer host"
			host := filepath.Join(root, "skgo")
			write(host, hostBytes)
			home := filepath.Join(root, "home")
			digest := fmt.Sprintf("%x", sha256.Sum256([]byte(hostBytes)))
			dir := filepath.Join(home, ".skgo", "plugins", "hosts", digest)
			slot := filepath.Join(dir, fmt.Sprintf("%x.so", sha256.Sum256([]byte("example.test/main"))))
			peer := filepath.Join(dir, fmt.Sprintf("%x.so", sha256.Sum256([]byte("example.test/peer"))))
			ref := strings.TrimSuffix(slot, ".so") + ".json"
			peerRef := strings.TrimSuffix(peer, ".so") + ".json"
			const oldRef = `{"module":"example.test/main","version":"v1.0.0"}`
			const peerSource = `{"module":"example.test/peer","version":"v1.0.0"}`
			write(slot, "working prior binary")
			write(ref, oldRef)
			write(peer, "working peer binary")
			write(peerRef, peerSource)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			buildFailure, probeFailure := errors.New("compiler refused fixture"), errors.New("candidate conflicts with peer")
			builds, probes, cleanups := 0, 0, 0
			var output bytes.Buffer
			sourceDir := filepath.Join(root, "source")
			o := Options{Host: buildinfo.Info{Executable: host, SkgoVersion: "v1.0.0", Build: &debug.BuildInfo{GoVersion: "go1.27.2", Main: debug.Module{Path: buildinfo.Module, Version: "v1.0.0"}}}, Home: home, CWD: root, Out: &output,
				fetchSource: func(_ context.Context, ref, version string, _ io.Writer) (source, func(), error) {
					if ref != "example.test/main@v1.1.0" || version != "go1.27.2" {
						t.Fatalf("source selection: %s %s", ref, version)
					}
					return source{resolution: resolution{Path: "example.test/main", Version: "v1.1.0"}, Root: sourceDir, Package: "."}, func() { cleanups++ }, nil
				},
				build: func(_ context.Context, opts pluginbuild.Options) error {
					builds++
					if opts.Host != host || opts.Source != sourceDir || opts.Package != "." || !opts.Generate || opts.VersionSymbol != "main.skgoVersion" {
						t.Fatalf("build options: %+v", opts)
					}
					if stage == "build failure" {
						return buildFailure
					}
					return os.WriteFile(opts.Output, []byte("qualified replacement"), 0755)
				},
				probe: func(_ context.Context, actualHost, candidate string, files []string) (Exports, error) {
					probes++
					lockContext, stopLock := context.WithTimeout(t.Context(), 10*time.Millisecond)
					defer stopLock()
					if unlock, lockErr := lock(lockContext, filepath.Join(dir, ".install.lock")); lockErr == nil {
						unlock()
						t.Fatal("qualification ran without the host installation lock")
					} else if !errors.Is(lockErr, context.DeadlineExceeded) {
						t.Fatalf("check qualification lock: %v", lockErr)
					}
					if actualHost != host || read(candidate) != "qualified replacement" {
						t.Fatal("wrong candidate passed to actual host probe")
					}
					if len(files) != 2 || !slices.Contains(files, peer) || !slices.Contains(files, candidate) || slices.Contains(files, slot) {
						t.Fatalf("qualification set: %v", files)
					}
					if read(slot) != "working prior binary" || read(ref) != oldRef {
						t.Fatal("published before qualification")
					}
					if stage == "probe failure" {
						return Exports{}, probeFailure
					}
					if stage == "canceled" {
						cancel()
					}
					return Exports{Templates: []string{"receipt-template"}, Addons: []string{"receipt-addon"}}, nil
				},
			}
			err = Install(ctx, "example.test/main@v1.1.0", o)
			lockContext, stopLock := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer stopLock()
			unlock, lockErr := lock(lockContext, filepath.Join(dir, ".install.lock"))
			if lockErr != nil {
				t.Fatalf("installation left its lock held: %v", lockErr)
			}
			unlock()
			wantProbes := 1
			if stage == "build failure" {
				wantProbes = 0
			}
			if builds != 1 || probes != wantProbes || cleanups != 1 {
				t.Fatalf("pipeline calls: builds=%d probes=%d cleanup=%d", builds, probes, cleanups)
			}
			if stage == "success" {
				if err != nil {
					t.Fatal(err)
				}
				if read(slot) != "qualified replacement" || read(ref) != `{"module":"example.test/main","version":"v1.1.0"}` {
					t.Fatal("qualified replacement was not published")
				}
				for _, want := range []string{"Installed example.test/main@v1.1.0", "Templates: receipt-template", "Add-ons: receipt-addon"} {
					if !strings.Contains(output.String(), want) {
						t.Fatalf("missing %q in %s", want, &output)
					}
				}
			} else {
				want := map[string]error{"build failure": buildFailure, "probe failure": probeFailure, "canceled": context.Canceled}[stage]
				if !errors.Is(err, want) {
					t.Fatalf("error=%v, want %v", err, want)
				}
				if read(slot) != "working prior binary" || read(ref) != oldRef {
					t.Fatal("failed installation changed prior selection")
				}
				if stage == "probe failure" && !strings.Contains(err.Error(), "skgo plugin remove example.test/peer") {
					t.Fatalf("missing peer removal hint: %v", err)
				}
			}
			if read(peer) != "working peer binary" || read(peerRef) != peerSource || read(host) != hostBytes {
				t.Fatal("installation changed its peer or host")
			}
		})
	}
}
