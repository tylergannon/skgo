package skgo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/tylergannon/polytype/devalue"
)

// PrerenderLoadInput carries one Kit build-time server-load invocation from the
// adapter's prerender process to the application's compiled Go load registry.
type PrerenderLoadInput struct {
	Module  string            `json:"module"`
	URL     string            `json:"url"`
	RouteID string            `json:"routeId"`
	Params  map[string]string `json:"params"`
	Headers http.Header       `json:"headers"`
	Parent  map[string]any    `json:"parent"`
}

type prerenderCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Path     string `json:"path"`
	Domain   string `json:"domain,omitempty"`
	MaxAge   int    `json:"maxAge,omitempty"`
	HTTPOnly bool   `json:"httpOnly"`
	Secure   bool   `json:"secure"`
	SameSite int    `json:"sameSite"`
}

type prerenderLoadOutput struct {
	Data     json.RawMessage    `json:"data,omitempty"`
	Chunks   []prerenderChunk   `json:"chunks,omitempty"`
	Error    *HTTPError         `json:"error,omitempty"`
	Failure  *string            `json:"failure,omitempty"`
	Redirect *prerenderRedirect `json:"redirect,omitempty"`
	Headers  http.Header        `json:"headers,omitempty"`
	Cookies  []prerenderCookie  `json:"cookies,omitempty"`
}

type prerenderChunk struct {
	ID    int             `json:"id"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

type prerenderRedirect struct {
	Status   int    `json:"status"`
	Location string `json:"location"`
}

// RunPrerenderLoad answers one load invocation without the final application
// manifest, which Kit has not written yet while it prerenders. The adapter
// calls a build-only Go executable that imports the generated registrations.
func RunPrerenderLoad(in io.Reader, out io.Writer, transport Transport, loads ...*ServerLoad) error {
	var input PrerenderLoadInput
	if err := json.NewDecoder(in).Decode(&input); err != nil {
		return fmt.Errorf("skgo: decode prerender load request: %w", err)
	}
	pageURL, err := url.Parse(input.URL)
	if err != nil || pageURL.Scheme == "" || pageURL.Host == "" {
		return fmt.Errorf("skgo: invalid prerender page URL %q", input.URL)
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, input.URL, nil)
	if err != nil {
		return err
	}
	request.Header = http.Header{}
	for name, values := range input.Headers {
		request.Header[http.CanonicalHeaderKey(name)] = append([]string(nil), values...)
	}
	registry, err := NewLoads(LoadConfig{Dev: true, Origin: pageURL.Scheme + "://" + pageURL.Host, Transport: transport}, loads...)
	if err != nil {
		return err
	}
	load := registry.byModule[input.Module]
	if load == nil {
		return fmt.Errorf("skgo: no generated Go load for %s", input.Module)
	}
	// Kit has already evaluated parent loads before calling this stub. A
	// synthetic preceding node lets skgo.Parent read the same merged values.
	parent := &ServerLoad{run: func(context.Context) (any, error) { return input.Parent, nil }}
	shared, nodes := registry.runBranchWith(request, dataRequest{url: pageURL}, input.RouteID, input.Params, []*ServerLoad{parent, load}, nil, nil, nil)
	node := nodes[1]
	answer := prerenderLoadOutput{Headers: shared.headers.Clone()}
	if node.redir != nil {
		answer.Redirect = &prerenderRedirect{Status: node.redir.status(), Location: node.redir.Location}
	} else if node.err != nil {
		answer.Error = node.err
		var authored *HTTPError
		if node.raw != nil && !errors.As(node.raw, &authored) {
			message := node.raw.Error()
			answer.Failure = &message
		}
	} else {
		promises := &promiseTable{ids: map[*deferred]int{}}
		tree, err := transport.encodeLoadValue(node.data)
		if err != nil {
			return fmt.Errorf("skgo: encode prerender load %s: %w", input.Module, err)
		}
		reducers := append(transport.reducers(), promiseReducer(promises))
		serialized, err := devalue.StringifyWith(tree, reducers)
		if err != nil {
			return fmt.Errorf("skgo: serialize prerender load %s: %w", input.Module, err)
		}
		answer.Data = json.RawMessage(serialized)
		var chunkErr error
		promises.settled(request.Context(), func(id int, value any, settleErr error) {
			chunk := prerenderChunk{ID: id}
			if settleErr != nil {
				chunk.Error = settleErr.Error()
			} else {
				tree, err := transport.encodeLoadValue(value)
				if err == nil {
					var encoded string
					encoded, err = devalue.StringifyWith(tree, reducers)
					chunk.Data = json.RawMessage(encoded)
				}
				if err != nil {
					chunkErr = fmt.Errorf("skgo: encode deferred value in %s: %w", input.Module, err)
				}
			}
			answer.Chunks = append(answer.Chunks, chunk)
		})
		if chunkErr != nil {
			return chunkErr
		}
	}
	for _, c := range shared.jar.snapshot() {
		answer.Cookies = append(answer.Cookies, prerenderCookie{
			Name: c.Name, Value: c.Value, Path: c.Path, Domain: c.Domain,
			MaxAge: c.MaxAge, HTTPOnly: c.HttpOnly, Secure: c.Secure, SameSite: int(c.SameSite),
		})
	}
	return json.NewEncoder(out).Encode(answer)
}
