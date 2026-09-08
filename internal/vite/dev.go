// Package vite is the Go half of Vite's module-runner protocol: a client for a
// running `vite dev` server, and a runner that evaluates the modules it
// transforms inside a goja runtime.
//
// It exists because `vite dev` never runs an adapter — kit reaches `adapt()`
// only from the plugin that finalises a build — so in dev there is no SSR
// bundle for the engine to evaluate. What there is instead is the same `goja`
// environment the build compiles, declared in the dev server by the same
// adapter plugin, and vite's own `fetchModule` over it. Go asks for one
// transformed module at a time and evaluates it in the engine the built bundle
// runs in; nothing else about the engine changes.
//
// The runner is a second implementation of something vite ships in JavaScript,
// and that is the cost. The contract it implements is six identifiers and one
// result shape (`vite/src/module-runner/constants.ts`), and everything on
// either side of it — the entry, the node table, the host bindings, the
// polyfill, the per-module lowering — is the build's, unchanged.
package vite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Dev is a running `vite dev` server, addressed over the endpoints the skgo
// adapter's dev plugin adds to it.
type Dev struct {
	base   string
	client *http.Client
}

// NewDev addresses the dev server at base, e.g. "http://127.0.0.1:5173".
func NewDev(base string) *Dev {
	return &Dev{
		base: strings.TrimSuffix(base, "/"),
		// No timeout: a cold `vite dev` compiles the module it is asked for,
		// and the first request for Svelte's server renderer can take seconds.
		client: &http.Client{},
	}
}

// Base is the dev server's URL.
func (d *Dev) Base() string { return d.base }

// Info is what the dev server says a document boots and where the render entry
// is. It is kit's own dev manifest `_.client`, which is not the build's: the
// client entry is served straight out of the installed package over `/@fs`,
// the app module out of the generated dev tree, and neither has an import,
// stylesheet or font beside it.
type Info struct {
	// Entry is the module the engine evaluates: the adapter's render entry, as
	// an absolute path the dev server resolves.
	Entry string `json:"entry"`
	// Client is what the boot script imports.
	Client Client `json:"client"`
	// GlobalName is the object the boot script assigns and kit's client reads.
	// In dev kit calls it `__sveltekit_dev` rather than `__sveltekit_<hash>`.
	GlobalName string `json:"globalName"`
}

// Client is kit's dev `manifest._.client`.
type Client struct {
	Start                string `json:"start"`
	App                  string `json:"app"`
	UsesEnvDynamicPublic bool   `json:"usesEnvDynamicPublic"`
}

// Module is one transformed module, exactly as vite's own `fetchModule`
// returns it.
type Module struct {
	// Code is the SSR transform's output: the module body, referring to its
	// imports through `__vite_ssr_import__` and its exports through
	// `__vite_ssr_exports__`.
	Code string `json:"code"`
	// File is the file on disk the module was compiled from, or "" for a
	// virtual module. It is the key an edit is matched against.
	File string `json:"file"`
	ID   string `json:"id"`
	URL  string `json:"url"`
	// Externalize is set when vite decided the module should be loaded by the
	// host's own module loader rather than transformed. The engine has none,
	// so it is a failure here rather than a mode.
	Externalize string `json:"externalize"`
	Type        string `json:"type"`
	// Error is the message the dev plugin caught while transforming.
	Error string `json:"error"`
}

// Await waits for the dev server to answer, and returns what it says. A Go
// process and a `vite dev` are started together and either may win.
func (d *Dev) Await(timeout time.Duration) (Info, error) {
	deadline := time.Now().Add(timeout)
	var last error
	for {
		info, err := d.Info()
		if err == nil {
			return info, nil
		}
		last = err
		if time.Now().After(deadline) {
			return Info{}, fmt.Errorf("skgo: the dev server at %s did not answer within %s: %w", d.base, timeout, last)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Info asks the dev server what a document boots and where the entry is.
func (d *Dev) Info() (Info, error) {
	var info Info
	if err := d.get("/__skgo_dev/info", &info); err != nil {
		return Info{}, err
	}
	if info.Entry == "" {
		return Info{}, fmt.Errorf("skgo: the dev server at %s named no render entry. Is the skgo adapter in its vite config?", d.base)
	}
	return info, nil
}

// Module fetches one transformed module. importer is the URL of the module
// that asked for it, or "" for the entry.
func (d *Dev) Module(url, importer string) (Module, error) {
	body, err := json.Marshal(map[string]string{"url": url, "importer": importer})
	if err != nil {
		return Module{}, err
	}
	resp, err := d.client.Post(d.base+"/__skgo_dev/module", "application/json", bytes.NewReader(body))
	if err != nil {
		return Module{}, fmt.Errorf("skgo: asking the dev server for %s: %w", url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Module{}, err
	}
	var out Module
	if err := json.Unmarshal(raw, &out); err != nil {
		return Module{}, fmt.Errorf("skgo: the dev server answered %d for %s with %s", resp.StatusCode, url, snippet(raw))
	}
	if out.Error != "" {
		return Module{}, fmt.Errorf("skgo: the dev server could not transform %s: %s", url, out.Error)
	}
	if out.Externalize != "" {
		return Module{}, fmt.Errorf("skgo: the dev server externalized %s as %s (%s); the engine has no module loader to reach it with",
			url, out.Externalize, out.Type)
	}
	return out, nil
}

// Changes is what the dev server has seen change since a cursor.
type Changes struct {
	// Version is the cursor to pass next time.
	Version int `json:"version"`
	// Reset reports a cursor from before this dev server started — a Go process
	// that outlived a vite restart. Nothing can be invalidated selectively
	// against it, so the caller starts over.
	Reset bool `json:"reset"`
	// Files are the files that changed, oldest first.
	Files []string `json:"files"`
}

// Changed asks what has changed since the given cursor.
func (d *Dev) Changed(since int) (Changes, error) {
	var out Changes
	if err := d.get("/__skgo_dev/changed?since="+strconv.Itoa(since), &out); err != nil {
		return Changes{}, err
	}
	return out, nil
}

// Get asks the dev server for one of the adapter plugin's own endpoints and
// decodes the JSON it answers with.
func (d *Dev) Get(path string, into any) error { return d.get(path, into) }

func (d *Dev) get(path string, into any) error {
	if _, err := url.Parse(d.base + path); err != nil {
		return err
	}
	resp, err := d.client.Get(d.base + path)
	if err != nil {
		return fmt.Errorf("skgo: asking the dev server for %s: %w", path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("skgo: the dev server answered %d for %s: %s", resp.StatusCode, path, snippet(raw))
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("skgo: the dev server answered %s with %s", path, snippet(raw))
	}
	return nil
}

func snippet(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if len(text) > 300 {
		return text[:300] + "…"
	}
	if text == "" {
		return "an empty body"
	}
	return text
}
