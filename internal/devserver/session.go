// Package devserver owns one local SKGO development session. The public
// listener remains here while the native application and Vite are replaceable.
package devserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/tylergannon/skgo/internal/gen"
	"github.com/tylergannon/skgo/internal/hotreload"
)

func Run(ctx context.Context, o Options) error {
	if o.Root == "" {
		o.Root = "."
	}
	if o.Output == nil {
		o.Output = os.Stdout
	}
	if o.GoTool == "" {
		o.GoTool = "go"
	}
	cfg, e := applicationConfig(o)
	if e != nil {
		return e
	}
	ln, e := listen(o.Listen, o.StrictPort)
	if e != nil {
		return e
	}
	address := ln.Addr().(*net.TCPAddr)
	host := address.IP.String()
	if address.IP.IsUnspecified() {
		host = "127.0.0.1"
	}
	origin := "http://" + net.JoinHostPort(host, fmt.Sprint(address.Port))
	// Vite can become ready before the first admitted Go host, including when
	// a captured startup build becomes obsolete. Keep public dispatch waiting.
	route := &router{requests: requestAdmission{ready: make(chan struct{})}}
	defer route.requests.close()
	server := &http.Server{Handler: route}
	defer server.Close()
	go server.Serve(ln)
	// Keep Unix socket paths below platform limits; the macOS temp root alone
	// can consume most of the available pathname. Development is Unix-only.
	work, e := os.MkdirTemp("/tmp", "skgo-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(work)
	work, e = filepath.EvalSymlinks(work)
	if e != nil {
		return e
	}
	project, e := gen.OpenProject(cfg)
	if e != nil {
		return e
	}
	defer project.Close()
	if o.GeneratorConfig != "" {
		config := o.GeneratorConfig
		if !filepath.IsAbs(config) {
			config = filepath.Join(o.Root, config)
		}
		config, e = filepath.Abs(config)
		if e != nil {
			return e
		}
		if e = project.WatchRawFile(config); e != nil {
			return e
		}
	}
	watchErrors, e := project.Watch()
	if e != nil {
		return e
	}
	var applicationGenerator *generator
	var rawChanges <-chan struct{}
	if o.GeneratorConfig != "" {
		applicationGenerator, e = newGenerator(o, work)
		if e != nil {
			return e
		}
		defer applicationGenerator.close()
		rawChanges = project.RawChanges()
		project.RecordRaw(true)
	}
	guardBuild := func(build *gen.ProjectBuild) {
		if applicationGenerator != nil {
			build.RequireEventEpoch(applicationGenerator.epoch)
		}
	}
	announcedPreparation := false
	for {
		if applicationGenerator != nil {
			if err := applicationGenerator.pass(ctx, project); errors.Is(err, gen.ErrObsolete) {
				continue
			} else if err != nil {
				return fmt.Errorf("application generator startup: %w", err)
			}
		}
		if !announcedPreparation {
			fmt.Fprintln(o.Output, "skgo dev: preparing generated application")
			announcedPreparation = true
		}
		initial, err := project.Prepare(ctx)
		if errors.Is(err, gen.ErrObsolete) {
			continue
		}
		if err != nil {
			return err
		}
		guardBuild(initial)
		err = initial.Commit(nil)
		initial.Inputs.Close()
		if errors.Is(err, gen.ErrObsolete) {
			continue
		}
		if err != nil {
			initial.Cancel()
			return err
		}
		break
	}
	fmt.Fprintln(o.Output, "skgo dev: starting Vite")
	secretBytes := make([]byte, 32)
	if _, e = rand.Read(secretBytes); e != nil {
		return e
	}
	secret := hex.EncodeToString(secretBytes)
	front, e := startFrontend(ctx, o, cfg.Web, work, origin, secret)
	if e != nil {
		return e
	}
	defer func() { front.close() }()
	if e = route.setFrontend(front.ready.Socket, front.ready.Tooling); e != nil {
		return e
	}
retryStartup:
	if applicationGenerator != nil {
		for {
			if e = applicationGenerator.pass(ctx, project); errors.Is(e, gen.ErrObsolete) {
				continue
			}
			if e != nil {
				return fmt.Errorf("application generator startup: %w", e)
			}
			break
		}
	}
	snapshot, e := front.snapshot(ctx)
	if e != nil {
		return e
	}
	route.setRemotePrefix(snapshot.RemotePrefix)
	if snapshot.Metadata != nil {
		if _, _, e = project.ObserveRoutes(snapshot.Metadata); e != nil {
			return e
		}
	}
	// Publish Go registrations discovered by Kit's installed route scanner.
	seed, e := project.Prepare(ctx)
	if e != nil {
		return e
	}
	guardBuild(seed)
	if e = seed.Commit(nil); e != nil {
		seed.Cancel()
		seed.Inputs.Close()
		if errors.Is(e, gen.ErrObsolete) {
			goto retryStartup
		}
		return e
	}
	seed.Inputs.Close()
	snapshot, e = front.snapshot(ctx)
	if e != nil {
		return e
	}
	if snapshot.Metadata != nil {
		if _, _, e = project.ObserveRoutes(snapshot.Metadata); e != nil {
			return e
		}
	}
	build, e := project.Prepare(ctx)
	if e != nil {
		return e
	}
	serial := 0
	type hostFiles struct{ dir, socket string }
	hostDirs := make(map[*hotreload.Host]hostFiles)
	// What a replaced host or revision leaves on disk is removed once the loop is
	// idle, after the browser has been told about the revision that replaced it.
	var sweep []func()
	stopHost := func(host *hotreload.Host, later bool) {
		files, owned := hostDirs[host]
		if !owned {
			return
		}
		_ = host.Close()
		delete(hostDirs, host)
		remove := func() {
			_ = os.Remove(files.socket)
			_ = os.RemoveAll(files.dir)
		}
		if later {
			sweep = append(sweep, remove)
		} else {
			remove()
		}
	}
	closeHost := func(host *hotreload.Host) { stopHost(host, false) }
	cache := &hotreload.HostCache{Dir: filepath.Join(work, "hosts")}
	// compileHost builds in the background; the returned join hands the host to
	// this loop, which alone owns the session's host bookkeeping.
	compileHost := func(ctx context.Context, inputs *gen.CompilerInputs) func() (*hotreload.Host, error) {
		serial++
		dir := filepath.Join(work, fmt.Sprintf("host-%d", serial))
		socket := filepath.Join(work, fmt.Sprintf("go-%d.sock", serial))
		var host *hotreload.Host
		var err error
		done := make(chan struct{})
		go func() {
			defer close(done)
			host, err = hotreload.CompileHost(ctx, hotreload.HostOptions{Inputs: inputs, WorkDir: dir, GoTool: o.GoTool, Origin: origin, SocketPath: socket, NativeKit: true, ServiceSecret: secret, Output: o.Output, Cache: cache})
		}()
		return func() (*hotreload.Host, error) {
			<-done
			if err != nil {
				_ = os.Remove(socket)
				_ = os.RemoveAll(dir)
				return nil, err
			}
			hostDirs[host] = hostFiles{dir, socket}
			return host, nil
		}
	}
	makeHost := func(inputs *gen.CompilerInputs, dist string) (*hotreload.Host, error) {
		host, err := compileHost(ctx, inputs)()
		if err != nil {
			return nil, err
		}
		if err := host.Start(ctx, dist); err != nil {
			closeHost(host)
			return nil, err
		}
		return host, nil
	}
	publishHost := func(host *hotreload.Host) error {
		// Vite dials a stable pathname; each candidate owns its own listener so
		// compilation cannot disturb the healthy application. Initialization runs
		// after its predecessor exits and releases exclusive application resources.
		socket := hostDirs[host].socket
		alias := filepath.Join(work, "go.sock")
		staged := alias + ".next"
		if err := os.Symlink(socket, staged); err != nil {
			return err
		}
		defer os.Remove(staged)
		if err := os.Rename(staged, alias); err != nil {
			return err
		}
		return route.setApp(socket)
	}
	fmt.Fprintln(o.Output, "skgo dev: building native application host")
	active, e := makeHost(build.Inputs, snapshot.Dist)
	if e != nil {
		message := build.Inputs.AuthoredText(e.Error())
		build.Cancel()
		build.Inputs.Close()
		if project.Revision() != build.Revision {
			goto retryStartup
		}
		return fmt.Errorf("Go development host: %s", message)
	}
	guardBuild(build)
	if e = build.Commit(func(*gen.CompilerInputs) error { return publishHost(active) }); e != nil {
		closeHost(active)
		build.Cancel()
		build.Inputs.Close()
		if errors.Is(e, gen.ErrObsolete) {
			goto retryStartup
		}
		return e
	}
	if applicationGenerator != nil {
		applicationGenerator.rebuild = false
	}
	hostInputs, goodInputs := build.Inputs, build.Inputs
	goodRevision := build.Revision
	goodDist := snapshot.Dist
	defer func() {
		closeHost(active)
		hostInputs.Close()
		if goodInputs != hostInputs {
			goodInputs.Close()
		}
		for _, remove := range sweep {
			remove()
		}
	}()
	route.setRemotePrefix(snapshot.RemotePrefix)
	route.requests.resume()
	fmt.Fprintf(o.Output, "skgo dev: %s (Go pid %d, Vite %s)\n", origin, active.PID, front.ready.Socket)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	lastFrontend := snapshot.Revision
	dirtyFrontend := false
	pending := false
	goChanged := false
	for {
		if !pending {
			for _, remove := range sweep {
				remove()
			}
			sweep = nil
			select {
			case <-ctx.Done():
				return nil
			case err := <-watchErrors:
				if err != nil {
					fmt.Fprintf(o.Output, "skgo dev watch: %v\n", err)
				}
				continue
			case <-rawChanges:
				pending = true
			case <-project.Changes():
				goChanged = true
				pending = true
			case <-active.Done():
				if ctx.Err() != nil {
					return nil
				}
				if err := route.requests.pause(ctx, requestDrain); err != nil {
					return nil
				}
				closeHost(active)
				next, err := makeHost(goodInputs, goodDist)
				if err != nil {
					if ctx.Err() != nil {
						return nil
					}
					return fmt.Errorf("recover Go application: %w", err)
				}
				closeHost(active)
				if hostInputs != goodInputs {
					hostInputs.Close()
				}
				active = next
				hostInputs = goodInputs
				if err := publishHost(active); err != nil {
					return err
				}
				route.requests.resume()
				_ = front.notify(ctx, goodRevision, "", false)
				fmt.Fprintf(o.Output, "skgo dev: recovered Go pid %d\n", active.PID)
				continue
			case <-front.done:
				front.close()
				if ctx.Err() != nil {
					return nil
				}
				nextFrontend, err := startFrontend(ctx, o, cfg.Web, work, origin, secret)
				if ctx.Err() != nil {
					if nextFrontend != nil {
						nextFrontend.close()
					}
					return nil
				}
				if err != nil {
					return err
				}
				front = nextFrontend
				if e = route.setFrontend(front.ready.Socket, front.ready.Tooling); e != nil {
					return e
				}
				dirtyFrontend = true
				pending = true
				lastFrontend = ""
			case <-ticker.C:
				if changed, err := front.refresh(); err == nil && changed {
					fmt.Fprintf(o.Output, "skgo dev: Vite restarted (instance %s, %s)\n", front.ready.Instance, front.ready.Socket)
					if err = route.setFrontend(front.ready.Socket, front.ready.Tooling); err != nil {
						return err
					}
					dirtyFrontend = true
					pending = true
					lastFrontend = ""
					continue
				}
				var state struct {
					Revision string `json:"revision"`
				}
				if e = front.request(ctx, "state", nil, &state); e != nil {
					continue
				}
				if state.Revision == lastFrontend {
					continue
				}
				lastFrontend = state.Revision
				dirtyFrontend = true
				pending = true
			}
		}
		pending = false
		if applicationGenerator != nil {
			if err := applicationGenerator.pass(ctx, project); err != nil {
				if errors.Is(err, gen.ErrObsolete) {
					pending = true
				} else {
					fmt.Fprintln(o.Output, err)
					_ = front.notify(ctx, project.Revision(), err.Error(), false)
				}
				continue
			}
			goChanged = goChanged || project.Revision() != goodRevision || applicationGenerator.rebuild
		}
		// A Go event can arrive before the frontend polling tick. Observe the
		// current frontend revision before deciding whether native installation
		// alone can publish this captured application revision.
		if !dirtyFrontend {
			var state struct {
				Revision string `json:"revision"`
			}
			if err := front.request(ctx, "state", nil, &state); err == nil && state.Revision != lastFrontend {
				dirtyFrontend = true
			}
		}
		if dirtyFrontend {
			next, err := front.snapshot(ctx)
			if err != nil {
				fmt.Fprintln(o.Output, err)
				_ = front.notify(ctx, project.Revision(), err.Error(), false)
				continue
			}
			snapshot = next
			lastFrontend = next.Revision
			changed := false
			if next.Metadata != nil {
				_, changed, err = project.ObserveRoutes(next.Metadata)
				if err != nil {
					return err
				}
			}
			if !changed && !goChanged && next.Dist == goodDist {
				dirtyFrontend = false
				continue
			}
		}
		nextBuild, err := project.Prepare(ctx)
		if err != nil {
			if errors.Is(err, gen.ErrObsolete) {
				pending = true
				continue
			}
			project.Diagnose(err)
			fmt.Fprintln(o.Output, err)
			_ = front.notify(ctx, project.Revision(), err.Error(), false)
			continue
		}
		guardBuild(nextBuild)
		serial++
		candidateDir := filepath.Join(work, fmt.Sprintf("candidate-%d", serial))
		prepareStarted := time.Now()
		var update *hotreload.HostUpdate
		rebuild := applicationGenerator != nil && applicationGenerator.rebuild
		if !rebuild {
			update, err = active.Prepare(ctx, nextBuild.Inputs, candidateDir)
		}
		prepareElapsed := time.Since(prepareStarted)
		var installElapsed time.Duration
		var complete *hotreload.CompleteBuild
		if errors.As(err, &complete) {
			rebuild = true
			fmt.Fprintf(o.Output, "skgo dev: rebuilding revision %d: %v\n", nextBuild.Revision, complete)
			err = nil
		}

		if err == nil && !rebuild {
			err = nextBuild.Commit(func(*gen.CompilerInputs) error {
				if update != nil {
					installStarted := time.Now()
					e := active.Install(ctx, update)
					installElapsed = time.Since(installStarted)
					if e != nil {
						return e
					}
				}
				if snapshot.Dist != goodDist {
					return active.Reload(ctx, snapshot.Dist)
				}
				return nil
			})
			if errors.As(err, &complete) {
				rebuild = true
				fmt.Fprintf(o.Output, "skgo dev: rebuilding revision %d after install admission: %v\n", nextBuild.Revision, complete)
				err = nil
			}
		}
		if err == nil && rebuild {
			// The host compiles from captured inputs while the new Go declarations
			// are published and Kit's registration configuration is captured; only
			// its start needs that snapshot.
			compileCtx, cancelCompile := context.WithCancel(ctx)
			compiled := compileHost(compileCtx, nextBuild.Inputs)
			if err = nextBuild.Publish(); err == nil {
				snapshot, err = front.snapshot(ctx)
				if err == nil && snapshot.Metadata != nil {
					var changed bool
					_, changed, err = project.ObserveRoutes(snapshot.Metadata)
					if err == nil && changed {
						// Regenerate Go callers if Kit discovered new route membership.
						dirtyFrontend = true
						err = gen.ErrObsolete
					}
				}
			}
			if err != nil {
				cancelCompile()
			}
			replacement, compileErr := compiled()
			cancelCompile()
			if err != nil && compileErr == nil {
				closeHost(replacement)
			}
			if err == nil {
				err = compileErr
			}
			if err == nil {
				old := active
				stopped := false
				paused := false
				// Commit refuses a revision that changed during the build.
				err = nextBuild.Commit(func(*gen.CompilerInputs) error {
					// Compilation and revision admission succeeded. Stop and wait before
					// any replacement constructor can claim application-owned resources.
					if err := route.requests.pause(ctx, requestDrain); err != nil {
						return err
					}
					paused = true
					stopHost(old, true)
					stopped = true
					if err := replacement.Start(ctx, snapshot.Dist); err != nil {
						return err
					}
					return publishHost(replacement)
				})
				if err != nil && stopped {
					closeHost(replacement)
					// Restore the last admitted inputs, including any installed native
					// edits newer than the previous process's original executable.
					recovered, recoveryErr := makeHost(goodInputs, goodDist)
					if recoveryErr == nil {
						recoveryErr = publishHost(recovered)
					}
					if recoveryErr != nil {
						if recovered != nil {
							closeHost(recovered)
						}
						return fmt.Errorf("replacement failed: %v; restore previous application: %w", err, recoveryErr)
					}
					active = recovered
					if hostInputs != goodInputs {
						hostInputs.Close()
					}
					hostInputs = goodInputs
					fmt.Fprintf(o.Output, "skgo dev: restored previous application (Go pid %d) after replacement failure\n", active.PID)
				}
				if err == nil {
					route.setRemotePrefix(snapshot.RemotePrefix)
					active = replacement
					closeHost(old)
					if hostInputs != goodInputs {
						sweep = append(sweep, hostInputs.Close)
					}
					hostInputs = nextBuild.Inputs
				} else {
					closeHost(replacement)
				}
				if paused {
					route.requests.resume()
				}
			}
		}
		sweep = append(sweep, func() { _ = os.RemoveAll(candidateDir) })
		if err != nil {
			if !errors.Is(err, gen.ErrObsolete) {
				nextBuild.Diagnose(err)
				message := nextBuild.Inputs.AuthoredText(err.Error())
				fmt.Fprintln(o.Output, message)
				_ = front.notify(ctx, nextBuild.Revision, message, false)
			} else {
				pending = true
			}
			nextBuild.Cancel()
			nextBuild.Inputs.Close()
			continue
		}
		if goodInputs != hostInputs {
			sweep = append(sweep, goodInputs.Close)
		}
		if rebuild && applicationGenerator != nil {
			applicationGenerator.rebuild = false
		}
		goodInputs = nextBuild.Inputs
		goodRevision = nextBuild.Revision
		goodDist = snapshot.Dist
		route.setRemotePrefix(snapshot.RemotePrefix)

		if err = front.notify(ctx, goodRevision, "", false); err != nil {
			fmt.Fprintf(o.Output, "skgo dev notify: %v\n", err)
		}
		dirtyFrontend = false
		goChanged = false

		fmt.Fprintf(o.Output, "skgo dev: published revision %d (Go pid %d, prepare %s, install %s, rebuild %t)\n", goodRevision, active.PID, prepareElapsed.Round(time.Microsecond), installElapsed.Round(time.Microsecond), rebuild)
	}
}
