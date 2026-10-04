package skgo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/tylergannon/polytype/devalue"
	"github.com/tylergannon/skgo/internal/remotearg"
)

// RunPrerenderBuild handles the two kinds of Go work Kit invokes while
// prerendering: a server load and a remote prerender function. It runs in the
// adapter's build-only Go command, before the final frontend manifest exists.
func RunPrerenderBuild(in io.Reader, out io.Writer, transport Transport, loads []*ServerLoad, remotes []*Remote) error {
	raw, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	var target struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &target); err != nil {
		return fmt.Errorf("skgo: decode prerender request: %w", err)
	}
	switch target.Kind {
	case "load":
		return RunPrerenderLoad(bytes.NewReader(raw), out, transport, loads...)
	case "remote":
		return runPrerenderRemote(raw, out, transport, remotes)
	case "remote-inputs":
		return runPrerenderInputs(raw, out, transport, remotes)
	default:
		return fmt.Errorf("skgo: unknown prerender request kind %q", target.Kind)
	}
}

// runPrerenderInputs evaluates a declared input producer without constructing
// a request or event. The serialized array is decoded by the adapter before
// Kit computes the canonical remote argument keys.
func runPrerenderInputs(raw []byte, out io.Writer, transport Transport, remotes []*Remote) error {
	var input struct {
		Kind   string `json:"kind"`
		Module string `json:"module"`
		Name   string `json:"name"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return fmt.Errorf("skgo: decode prerender remote inputs: %w", err)
	}
	if input.Kind != "remote-inputs" {
		return fmt.Errorf("skgo: invalid prerender inputs request kind %q", input.Kind)
	}
	var fn *Remote
	for _, remote := range remotes {
		if remote.module == input.Module && remote.name == input.Name {
			fn = remote
			break
		}
	}
	if fn == nil || fn.kind != KindPrerender || fn.inputs == nil {
		return fmt.Errorf("skgo: no generated Go prerender inputs for %s#%s", input.Module, input.Name)
	}
	values, err := fn.inputs(context.Background(), Call{transport: transport})
	if err != nil {
		return fmt.Errorf("skgo: prerender inputs %s#%s: %w", input.Module, input.Name, err)
	}
	encoded, err := devalue.StringifyWith(values, transport.reducers())
	if err != nil {
		return fmt.Errorf("skgo: serialize prerender inputs %s#%s: %w", input.Module, input.Name, err)
	}
	return json.NewEncoder(out).Encode(struct {
		Inputs string `json:"inputs"`
	}{Inputs: encoded})
}

func runPrerenderRemote(raw []byte, out io.Writer, transport Transport, remotes []*Remote) error {
	var input struct {
		Module  string      `json:"module"`
		Name    string      `json:"name"`
		Payload string      `json:"payload"`
		URL     string      `json:"url"`
		Headers http.Header `json:"headers"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return fmt.Errorf("skgo: decode prerender remote: %w", err)
	}
	pageURL, err := url.Parse(input.URL)
	if err != nil || pageURL.Scheme == "" || pageURL.Host == "" {
		return fmt.Errorf("skgo: invalid prerender page URL %q", input.URL)
	}
	request, err := http.NewRequest(http.MethodGet, input.URL, nil)
	if err != nil {
		return err
	}
	request.Header = input.Headers.Clone()
	registry, err := NewRemotes(RemoteConfig{Dev: true, Origin: pageURL.Scheme + "://" + pageURL.Host, Transport: transport}, remotes...)
	if err != nil {
		return err
	}
	var fn *Remote
	for _, remote := range remotes {
		if remote.module == input.Module && remote.name == input.Name {
			fn = remote
			break
		}
	}
	if fn == nil || fn.kind != KindPrerender {
		return fmt.Errorf("skgo: no generated Go prerender remote for %s#%s", input.Module, input.Name)
	}
	arg, present, err := remotearg.ParsePayloadWith(input.Payload, registry.codecs())
	if err != nil {
		return fmt.Errorf("skgo: decode prerender argument for %s: %w", fn.id, err)
	}
	value, err, panicked, diagnostic := callPrerenderRemote(registry, fn, withEvent(request.Context(), registry.newEvent(request, false)), registry.newCall(arg, present))
	if err != nil {
		if redirect := asRedirect(err); redirect != nil {
			return json.NewEncoder(out).Encode(struct {
				Type     string `json:"type"`
				Redirect struct {
					Status   int    `json:"status"`
					Location string `json:"location"`
				} `json:"redirect"`
			}{Type: "redirect", Redirect: struct {
				Status   int    `json:"status"`
				Location string `json:"location"`
			}{Status: redirect.status(), Location: redirect.Location}})
		}
		kind := "unknown"
		var authored *HTTPError
		if !panicked && errors.As(err, &authored) {
			kind = "app"
		}
		response := struct {
			Type       string     `json:"type"`
			Kind       string     `json:"kind"`
			Error      *HTTPError `json:"error"`
			Diagnostic *string    `json:"diagnostic,omitempty"`
		}{Type: "error", Kind: kind, Error: asHTTPError(err)}
		if kind == "unknown" {
			response.Diagnostic = &diagnostic
		}
		return json.NewEncoder(out).Encode(response)
	}
	data, err := devalue.StringifyWith(map[string]any{"_": value}, transport.reducers())
	if err != nil {
		return fmt.Errorf("skgo: serialize prerender remote %s: %w", fn.id, err)
	}
	return json.NewEncoder(out).Encode(struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}{Type: "result", Data: data})
}

// callPrerenderRemote preserves whether an error came from an authored Go
// error or a recovered panic. The production remote path intentionally folds
// both to an HTTP 500; Kit's build crawler needs the origin so its own native
// handleError path can distinguish app errors from unknown throws.
func callPrerenderRemote(registry *Remotes, fn *Remote, ctx context.Context, call Call) (value any, err error, panicked bool, diagnostic string) {
	defer func() {
		if recovered := recover(); recovered != nil {
			panicked = true
			diagnostic = fmt.Sprint(recovered)
			err = registry.recovered(fn, recovered, nil)
		}
	}()
	value, err = fn.call(ctx, call)
	if err != nil {
		diagnostic = err.Error()
	}
	return
}
