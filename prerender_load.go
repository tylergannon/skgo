package skgo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/tylergannon/devalue/v5"
)

// PrerenderLoadInput carries one Kit build-time server-load invocation from the
// adapter's prerender process to the application's compiled Go load registry.
type PrerenderLoadInput struct {
	Module       string            `json:"module"`
	URL          string            `json:"url"`
	RouteID      string            `json:"routeId"`
	Params       map[string]string `json:"params"`
	Headers      http.Header       `json:"headers"`
	Parent       map[string]any    `json:"parent"`
	ParentData   string            `json:"parentData,omitempty"`
	RoutePattern string            `json:"routePattern,omitempty"`
	RouteParams  []ManifestParam   `json:"routeParams,omitempty"`
	RoutePath    string            `json:"routePath,omitempty"`
}

type prerenderCookie struct {
	Name        string     `json:"name"`
	Value       string     `json:"value"`
	Path        string     `json:"path"`
	Domain      string     `json:"domain,omitempty"`
	MaxAge      *int       `json:"maxAge,omitempty"`
	Expires     *time.Time `json:"expires,omitempty"`
	Partitioned bool       `json:"partitioned,omitempty"`
	Priority    string     `json:"priority,omitempty"`
	Raw         bool       `json:"raw,omitempty"`
	HTTPOnly    bool       `json:"httpOnly"`
	Secure      bool       `json:"secure"`
	SameSite    int        `json:"sameSite"`
}

type prerenderLoadOutput struct {
	NativeError *nativeErrorDiagnostic `json:"nativeError,omitempty"`
	Body        json.RawMessage        `json:"body,omitempty"`
	Data        json.RawMessage        `json:"data,omitempty"`
	Chunks      []prerenderChunk       `json:"chunks,omitempty"`
	Error       *HTTPError             `json:"error,omitempty"`
	Failure     *string                `json:"failure,omitempty"`
	Redirect    *prerenderRedirect     `json:"redirect,omitempty"`
	Headers     http.Header            `json:"headers,omitempty"`
	Cookies     []prerenderCookie      `json:"cookies,omitempty"`
	Uses        usesWire               `json:"uses"`
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

// runPrerenderLoad executes exactly the load Kit requested, with Kit's parent
// data, through the same load engine that serves application requests.
func runPrerenderLoad(ctx context.Context, in io.Reader, out io.Writer, transport Transport, loads []*ServerLoad) error {
	var input PrerenderLoadInput
	if err := json.NewDecoder(in).Decode(&input); err != nil {
		return fmt.Errorf("skgo: decode prerender load request: %w", err)
	}
	if input.ParentData != "" {
		var err error
		input.Parent, err = decodePrerenderParent(input.ParentData, transport)
		if err != nil {
			return err
		}
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
	options, _ := ctx.Value(prerenderOptionsKey{}).(PrerenderServiceOptions)
	registry, err := NewLoads(LoadConfig{Origin: pageURL.Scheme + "://" + pageURL.Host, Transport: transport, HandleError: options.HandleError}, loads...)
	if err != nil {
		return err
	}
	if development, ok := ctx.Value(devRendererKey{}).(*SSR); ok {
		registry = development.loads
	}
	load := registry.byModule[input.Module]
	if load == nil {
		return fmt.Errorf("skgo: no generated Go load for %s", input.Module)
	}
	// This node is skipped unless the actual Go load calls Parent. In dev its
	// callback asks Kit for parent data; build prerender supplies it up front.
	parent := &ServerLoad{run: func(context.Context) (any, error) {
		if parent, ok := ctx.Value(devParentKey{}).(func() (map[string]any, error)); ok {
			return parent()
		}
		return input.Parent, nil
	}}
	var converted map[string]any
	if state := hookStateOf(ctx); state != nil {
		input.Params, converted = state.params, state.converted
	}
	if input.RoutePattern != "" && hookStateOf(ctx) == nil {
		pattern, err := regexp.Compile(kitPattern(input.RoutePattern))
		if err != nil {
			return fmt.Errorf("skgo: prerender route pattern: %w", err)
		}
		match := pattern.FindStringSubmatchIndex(input.RoutePath)
		if match == nil {
			return fmt.Errorf("skgo: prerender path %q does not match route %s", input.RoutePath, input.RouteID)
		}
		var accepted bool
		input.Params, converted, accepted = execMatchedParams(input.RoutePath, match, input.RouteParams, registry.matchers)
		if !accepted {
			return fmt.Errorf("skgo: Go matchers reject Kit prerender route %s at %s", input.RouteID, input.RoutePath)
		}
	}
	var previousHeaders http.Header
	if ctx.Value(devRendererKey{}) == nil {
		previousHeaders = requestResponseHeaders(request).Clone()
	}
	shared, nodes := registry.runBranchWith(request, dataRequest{url: pageURL, converted: converted}, input.RouteID, input.Params, []*ServerLoad{parent, load}, []bool{false, true}, nil, nil)
	node := nodes[1]
	answer := prerenderLoadOutput{Uses: node.uses.serialize()}
	if _, development := ctx.Value(devRendererKey{}).(*SSR); development {
		answer.Headers = developmentHeaders(ctx, shared.responseState)
	} else {
		answer.Headers = shared.headers.Clone()
		for name := range previousHeaders {
			delete(answer.Headers, name)
		}
	}
	if node.redir != nil {
		answer.Redirect = &prerenderRedirect{Status: node.redir.status(), Location: node.redir.Location}
	} else if node.err != nil {
		answer.Error = node.err
		if development, ok := ctx.Value(devRendererKey{}).(*SSR); ok {
			handled := development.loads.dataError(ctx, input.RouteID, node.err, node.raw)
			body, err := jsonBytes(errorNodeExtra(handled))
			if err != nil {
				return err
			}
			answer.Body = body
			answer.NativeError = nativeErrorFor(ctx, node.raw)
			answer.Error = &HTTPError{Status: handled.Status, Message: handled.Message}
		} else if options.HandleError != nil {
			handled := registry.dataError(hookContext(request, shared), input.RouteID, node.err, node.raw)
			answer.Body, err = json.Marshal(handled)
			if err != nil {
				return err
			}
			answer.Error = &HTTPError{Status: handled.Status, Message: handled.Message}
		}
		var authored *HTTPError
		if node.raw != nil && !errors.As(node.raw, &authored) && ctx.Value(devRendererKey{}) == nil && options.HandleError == nil {
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
		streaming, _ := ctx.Value(devStreamingLoadKey{}).(bool)
		if streaming {
			answer.Cookies = prerenderCookies(shared.jar)
			if err := json.NewEncoder(out).Encode(answer); err != nil {
				return err
			}
			if flush, ok := out.(http.Flusher); ok {
				flush.Flush()
			}
		}
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
			if streaming {
				if err := json.NewEncoder(out).Encode(chunk); err != nil {
					chunkErr = err
				}
				if flush, ok := out.(http.Flusher); ok {
					flush.Flush()
				}
			} else {
				answer.Chunks = append(answer.Chunks, chunk)
			}
		})
		if chunkErr != nil {
			return chunkErr
		}
		if streaming {
			return request.Context().Err()
		}
	}
	answer.Cookies = prerenderCookies(shared.jar)
	return json.NewEncoder(out).Encode(answer)
}

func decodePrerenderParent(data string, transport Transport) (map[string]any, error) {
	decoded, err := devalue.Parse(data, transport.revivers())
	if err != nil {
		return nil, fmt.Errorf("skgo: decode native parent data: %w", err)
	}
	object, ok := decoded.(*devalue.Object)
	if !ok {
		return nil, fmt.Errorf("skgo: native parent data is not an object")
	}
	parent := map[string]any{}
	for _, key := range object.Keys() {
		parent[key], _ = object.Get(key)
	}
	return parent, nil
}
