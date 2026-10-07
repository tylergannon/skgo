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
)

// runPrerenderInputs evaluates a declared input producer without constructing
// a request or event. The serialized array is decoded by the adapter before
// Kit computes the canonical remote argument keys.
func runPrerenderInputs(ctx context.Context, raw []byte, out io.Writer, transport Transport, remotes []*Remote) error {
	var input struct {
		Module string `json:"module"`
		Name   string `json:"name"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return fmt.Errorf("skgo: decode prerender remote inputs: %w", err)
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
	values, err := callPrerenderInputs(ctx, fn, Call{transport: transport})
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

func runPrerenderRemote(ctx context.Context, raw []byte, out io.Writer, transport Transport, remotes []*Remote) error {
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
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return err
	}
	if original, ok := ctx.Value(prerenderRequestKey{}).(*http.Request); ok {
		request = original.WithContext(ctx)
	}
	request.Header = http.Header{}
	for name, values := range input.Headers {
		request.Header[http.CanonicalHeaderKey(name)] = append([]string(nil), values...)
	}
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
	result := registry.invokePayload(withEvent(request.Context(), registry.newEvent(request, false)), fn, input.Payload)
	if result.argumentError {
		return fmt.Errorf("skgo: decode prerender argument for %s: %s", fn.id, result.diagnostic)
	}
	value, err, panicked, diagnostic := result.value, result.err, result.panicked, result.diagnostic
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
	data, err := encodeRemoteResult(transport, map[string]any{"_": value})
	if err != nil {
		return fmt.Errorf("skgo: serialize prerender remote %s: %w", fn.id, err)
	}
	return json.NewEncoder(out).Encode(struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}{Type: "result", Data: data})
}

// Declared inputs have no application event, but share the request cancellation.
func callPrerenderInputs(ctx context.Context, fn *Remote, call Call) (values []any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("input producer panicked: %v", recovered)
		}
	}()
	return fn.inputs(ctx, call)
}
