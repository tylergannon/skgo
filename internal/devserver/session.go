// Package devserver owns one local SKGO development session. The public
// listener remains here while the native application and Vite are replaceable.
package devserver

import (
	"context"
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
	route := &router{}
	route.status("Starting Go and Vite")
	server := &http.Server{Handler: route}
	defer server.Close()
	go server.Serve(ln)
	work, e := os.MkdirTemp("", "skgo-dev-")
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
	watchErrors, e := project.Watch()
	if e != nil {
		return e
	}
	fmt.Fprintln(o.Output, "skgo dev: preparing generated application")
	for {
		initial, err := project.Prepare(ctx)
		if errors.Is(err, gen.ErrObsolete) {
			continue
		}
		if err != nil {
			return err
		}
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
	front, e := startFrontend(ctx, o, cfg.Web, filepath.Join(work, "vite"), origin)
	if e != nil {
		return e
	}
	defer func() { front.close() }()
	if e = route.setFrontend(front.ready.URL, front.ready.Tooling); e != nil {
		return e
	}
retryStartup:
	snapshot, e := front.snapshot(ctx)
	if e != nil {
		return e
	}
	if snapshot.Metadata != nil {
		if _, _, e = project.ObserveRoutes(snapshot.Metadata); e != nil {
			return e
		}
	}
	// Publish generated registrations before the frontend builds the paired graph.
	seed, e := project.Prepare(ctx)
	if e != nil {
		return e
	}
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
	hostDirs := make(map[*hotreload.Host]string)
	closeHost := func(host *hotreload.Host) {
		_ = host.Close()
		_ = os.RemoveAll(hostDirs[host])
		delete(hostDirs, host)
	}
	makeHost := func(inputs *gen.CompilerInputs, dist string) (*hotreload.Host, error) {
		serial++
		dir := filepath.Join(work, fmt.Sprintf("host-%d", serial))
		host, err := hotreload.BuildHost(ctx, hotreload.HostOptions{Inputs: inputs, WorkDir: dir, GoTool: o.GoTool, Origin: origin, DistDir: dist, Output: o.Output})
		if err != nil {
			_ = os.RemoveAll(dir)
			return nil, err
		}
		hostDirs[host] = dir
		return host, nil
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
	if e = build.Commit(func(*gen.CompilerInputs) error { return route.setApp(active.URL) }); e != nil {
		closeHost(active)
		build.Cancel()
		build.Inputs.Close()
		if errors.Is(e, gen.ErrObsolete) {
			goto retryStartup
		}
		return e
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
	}()
	route.status("")
	fmt.Fprintf(o.Output, "skgo dev: %s (Go pid %d, Vite %s)\n", origin, active.PID, front.ready.URL)
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	lastFrontend := snapshot.Revision
	dirtyFrontend := false
	graphChanged := false
	pending := false
	for {
		if !pending {
			select {
			case <-ctx.Done():
				return nil
			case err := <-watchErrors:
				if err != nil {
					fmt.Fprintf(o.Output, "skgo dev watch: %v\n", err)
				}
				continue
			case <-project.Changes():
				pending = true
			case <-active.Done():
				route.status("Restarting the Go application")
				next, err := makeHost(goodInputs, goodDist)
				if err != nil {
					return fmt.Errorf("recover Go application: %w", err)
				}
				closeHost(active)
				if hostInputs != goodInputs {
					hostInputs.Close()
				}
				active = next
				hostInputs = goodInputs
				_ = route.setApp(active.URL)
				route.status("")
				_ = front.notify(ctx, goodRevision, "", false)
				fmt.Fprintf(o.Output, "skgo dev: recovered Go pid %d\n", active.PID)
				continue
			case <-front.done:
				route.status("Restarting Vite")
				front.close()
				front, e = startFrontend(ctx, o, cfg.Web, filepath.Join(work, "vite"), origin)
				if e != nil {
					return e
				}
				if e = route.setFrontend(front.ready.URL, front.ready.Tooling); e != nil {
					return e
				}
				dirtyFrontend = true
				pending = true
				lastFrontend = ""
			case <-ticker.C:
				if changed, err := front.refresh(); err == nil && changed {
					fmt.Fprintf(o.Output, "skgo dev: Vite restarted (instance %s, %s)\n", front.ready.Instance, front.ready.URL)
					if err = route.setFrontend(front.ready.URL, front.ready.Tooling); err != nil {
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
				route.status(err.Error())
				fmt.Fprintln(o.Output, err)
				_ = front.notify(ctx, project.Revision(), err.Error(), false)
				continue
			}
			snapshot = next
			lastFrontend = next.Revision
			if next.Metadata != nil {
				var changed bool
				if _, changed, err = project.ObserveRoutes(next.Metadata); err == nil && changed {
					graphChanged = true
					route.blocked.Store(true)
				}
				if err != nil {
					return err
				}
			}
		}
		nextBuild, err := project.Prepare(ctx)
		if err != nil {
			if errors.Is(err, gen.ErrObsolete) {
				pending = true
				continue
			}
			project.Diagnose(err)
			route.status(err.Error())
			fmt.Fprintln(o.Output, err)
			_ = front.notify(ctx, project.Revision(), err.Error(), false)
			continue
		}
		serial++
		candidateDir := filepath.Join(work, fmt.Sprintf("candidate-%d", serial))
		prepareStarted := time.Now()
		update, err := active.Prepare(ctx, nextBuild.Inputs, candidateDir)
		prepareElapsed := time.Since(prepareStarted)
		var installElapsed time.Duration
		rebuild := false
		var complete *hotreload.CompleteBuild
		if errors.As(err, &complete) {
			rebuild = true
			fmt.Fprintf(o.Output, "skgo dev: rebuilding revision %d: %v\n", nextBuild.Revision, complete)
			err = nil
		}
		if err == nil && update != nil && dirtyFrontend {
			rebuild = true
			fmt.Fprintf(o.Output, "skgo dev: rebuilding revision %d: Go and renderer changed together\n", nextBuild.Revision)
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
				if dirtyFrontend {
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
			// New declarations must reach Kit's transform before making its artifact.
			if err = nextBuild.Commit(nil); err == nil {
				snapshot, err = front.snapshot(ctx)
				if err == nil && snapshot.Metadata != nil {
					var changed bool
					_, changed, err = project.ObserveRoutes(snapshot.Metadata)
					if err == nil && changed {
						// Kit may discover a route before the polling tick. Admit that
						// graph and regenerate its Go callers before building a host.
						graphChanged = true
						dirtyFrontend = true
						route.blocked.Store(true)
						err = gen.ErrObsolete
					}
				}
				if err == nil {
					var replacement *hotreload.Host
					replacement, err = makeHost(nextBuild.Inputs, snapshot.Dist)
					if err == nil {
						// A new lease keeps replacement publication guarded after a long build.
						current, prepareErr := project.Prepare(ctx)
						if prepareErr != nil {
							closeHost(replacement)
							err = prepareErr
						} else if current.Revision != nextBuild.Revision {
							current.Cancel()
							closeHost(replacement)
							err = gen.ErrObsolete
						} else {
							err = current.Commit(func(*gen.CompilerInputs) error { return route.setApp(replacement.URL) })
							current.Inputs.Close()
							if err == nil {
								old := active
								active = replacement
								closeHost(old)
								if hostInputs != goodInputs {
									hostInputs.Close()
								}
								hostInputs = nextBuild.Inputs
							} else {
								closeHost(replacement)
							}
						}
					}
				}
			}
		}
		_ = os.RemoveAll(candidateDir)
		if err != nil {
			if !errors.Is(err, gen.ErrObsolete) {
				nextBuild.Diagnose(err)
				message := nextBuild.Inputs.AuthoredText(err.Error())
				route.status(message)
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
			goodInputs.Close()
		}
		goodInputs = nextBuild.Inputs
		goodRevision = nextBuild.Revision
		goodDist = snapshot.Dist
		route.status("")
		route.blocked.Store(false)
		if err = front.notify(ctx, goodRevision, "", graphChanged); err != nil {
			fmt.Fprintf(o.Output, "skgo dev notify: %v\n", err)
		}
		dirtyFrontend = false
		graphChanged = false
		fmt.Fprintf(o.Output, "skgo dev: published revision %d (Go pid %d, prepare %s, install %s, rebuild %t)\n", goodRevision, active.PID, prepareElapsed.Round(time.Microsecond), installElapsed.Round(time.Microsecond), rebuild)
	}
}
