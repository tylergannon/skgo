// The routing half of a manifest, under `vp dev`, where it comes from the
// running dev server rather than from the last build.
//
// A build's `skgo.manifest.json` describes a frontend that has stopped moving.
// A dev server's route tree is a directory a developer is editing while both
// servers run, and every number in it moves with them: kit numbers a node by
// walking `src/routes` — layouts and error pages first, then leaves, in
// traversal order (`core/sync/create_manifest_data/index.js`) — so a page
// added anywhere renumbers every leaf after it. A route table read once at
// startup is therefore not merely missing the new route: it points the old
// routes at the wrong nodes, and the wrong page renders at HTTP 200.
//
// Kit answers this by rebuilding its manifest from the routes on disk whenever
// one appears or disappears (`exports/vite/dev/index.js`, `update_manifest`).
// Go asks the same dev server for the same answer.
package skgo

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/tylergannon/skgo/internal/vite"
)

// devRouting is what the adapter's dev plugin says the routing is. It is the
// routing fields of a build manifest, in kit's dev numbering, plus the cursor
// that makes asking again cheap.
type devRouting struct {
	// Version identifies the route tree this answer describes. Passing it back
	// asks "has anything changed since", which is answered without recomputing
	// anything.
	Version int `json:"version"`
	// Changed is false for an answer that carries nothing but the cursor.
	Changed  bool              `json:"changed"`
	Nodes    []string          `json:"nodes"`
	SSRNodes []ManifestSSRNode `json:"ssrNodes"`
	Routes   []ManifestRoute   `json:"routes"`
	// Error is what the dev plugin caught while reading the route tree — an
	// invalid route id, most often. Kit shows the same failure as an overlay
	// and keeps serving with the manifest it had.
	Error string `json:"error"`
}

// DevManifest keeps the route tables Go serves with describing what `vite dev`
// is serving.
//
// It holds no routing of its own: it owns the manifest, and the registries and
// the renderer it is given follow it. Rebuilding them is cheap — a route table
// is a list of compiled patterns over registrations that never change — and it
// happens only when the dev server says the route tree moved, which is once per
// file a developer adds or removes.
type DevManifest struct {
	dev   *vite.Dev
	build Manifest
	logf  func(format string, args ...any)

	mu        sync.Mutex
	version   int
	current   Manifest
	loads     *Loads
	endpoints *Endpoints
	ssr       *SSR
}

// NewDevManifest reads the routing half of the manifest out of a running
// `vite dev`, waiting for it to come up. build is the manifest of the embedded
// frontend, which is still where everything that is not routing comes from:
// kit's `appDir` and `base`, the document templates, the app's CSP.
func NewDevManifest(devServer string, build Manifest, logf func(format string, args ...any)) (*DevManifest, error) {
	if build.SSR == nil {
		return nil, errors.New("skgo: this build has no SSR description, so dev has no node table to render a branch through. Rebuild the frontend with an adapter that emits one.")
	}
	d := &DevManifest{dev: vite.NewDev(devServer), build: build, logf: logf, current: build}

	deadline := time.Now().Add(devServerTimeout)
	for {
		answer, err := d.ask(-1)
		if err == nil {
			if err := d.adopt(answer); err != nil {
				return nil, err
			}
			return d, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Manifest is the manifest as the dev server last described it.
func (d *DevManifest) Manifest() Manifest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.current
}

// Intercept keeps the registries and the renderer below it in step with the dev
// server, and passes every request on.
//
// It sits outside all of them because routing is the first thing every one of
// them does: the loads registry matches `__data.json` against it, the server
// routes match a path against it, and the renderer matches a document request
// against it and then resolves the branch's node indices through it. There is
// no request that may see one of those tables and not the others.
func (d *DevManifest) Intercept(loads *Loads, endpoints *Endpoints, ssr *SSR, next http.Handler) http.Handler {
	d.mu.Lock()
	d.loads, d.endpoints, d.ssr = loads, endpoints, ssr
	err := d.apply(d.current)
	d.mu.Unlock()
	if err != nil {
		// The manifest built these registries a moment ago, so this is not a
		// routing failure; say so and serve.
		d.report("skgo: dev routing: %v", err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.sync()
		next.ServeHTTP(w, r)
	})
}

// sync asks the dev server whether the route tree has moved and, if it has,
// rebuilds the tables over the new one.
//
// It runs per request rather than on a timer or a push because that is the only
// schedule with no window in it: a developer who saves a route and reloads the
// page is asking about the tree as it is now, and a poll interval is a period
// in which the answer is knowably wrong. The cost is one request to a server on
// the same machine that answers an unchanged tree with a cursor and nothing
// else — the engine already makes one of those per render for the same reason.
func (d *DevManifest) sync() {
	d.mu.Lock()
	defer d.mu.Unlock()

	answer, err := d.ask(d.version)
	if err != nil {
		d.report("%v", err)
		return
	}
	if !answer.Changed {
		return
	}
	if err := d.adopt(answer); err != nil {
		// The tables in place are the last ones that built, which is what kit
		// does with a route tree it cannot read.
		d.report("skgo: the dev server's route tree was refused, so the routing in place is the one before it: %v", err)
	}
}

// adopt makes an answer the current manifest. The caller holds the lock.
func (d *DevManifest) adopt(answer devRouting) error {
	m := d.build
	m.Nodes = answer.Nodes
	m.Routes = answer.Routes
	ssr := *d.build.SSR
	ssr.Nodes = answer.SSRNodes
	m.SSR = &ssr

	if len(m.SSR.Nodes) != len(m.Nodes) {
		return fmt.Errorf("skgo: the dev server describes %d node(s) for rendering and %d for loading", len(m.SSR.Nodes), len(m.Nodes))
	}
	if err := d.apply(m); err != nil {
		return err
	}
	d.current = m
	d.version = answer.Version
	return nil
}

// apply pushes a manifest's routing into whatever is following it.
func (d *DevManifest) apply(m Manifest) error {
	if d.loads != nil {
		if err := d.loads.setRouting(m.Nodes, m.Routes); err != nil {
			return err
		}
		d.reportMissingLoads(m)
	}
	if d.endpoints != nil {
		if err := d.endpoints.setRouting(m.Routes); err != nil {
			return err
		}
	}
	if d.ssr != nil {
		d.ssr.setNodes(m.SSR.Nodes)
	}
	return nil
}

// reportMissingLoads names a route whose `+*.server.ts` no Go load answers.
//
// In a build this is fatal — `NewLoads` refuses a binary that does not answer
// the frontend it was handed. In dev it cannot be, because the frontend moves
// and the binary does not: a developer who has just written a `page.server.go`
// has to run `go generate` and rebuild before Go can answer it. Saying so is
// the difference between that and a page that renders with no data.
func (d *DevManifest) reportMissingLoads(m Manifest) {
	for _, module := range m.Nodes {
		if module == "" {
			continue
		}
		if _, ok := d.loads.byModule[module]; !ok {
			d.report("skgo: %s has no Go load in this binary; run `go generate ./...` and restart it", module)
		}
	}
}

// ask fetches the routing. A negative cursor asks for the whole answer, which
// is what a process that has not seen one yet needs.
func (d *DevManifest) ask(since int) (devRouting, error) {
	path := "/__skgo_dev/manifest"
	if since >= 0 {
		path += "?since=" + strconv.Itoa(since)
	}
	var answer devRouting
	if err := d.dev.Get(path, &answer); err != nil {
		return devRouting{}, err
	}
	if answer.Error != "" {
		return devRouting{}, fmt.Errorf("skgo: the dev server could not read the route tree: %s", answer.Error)
	}
	return answer, nil
}

func (d *DevManifest) report(format string, args ...any) {
	if d.logf != nil {
		d.logf(format, args...)
	}
}
