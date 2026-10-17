package skgo

import (
	"context"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// Native internal fetches enter Kit before application middleware. Kit owns
// page/data dispatch and starts a fresh Go request boundary through its hooks.
type devKitTransportKey struct{}

func (d *NativeDevelopment) kitFetch(ctx context.Context, source *http.Request, request *http.Request) (*http.Response, error) {
	depth := fetchDepthOf(ctx) + 1
	if depth > maxFetchDepth {
		return nil, fmt.Errorf("skgo: native internal fetch exceeded maximum depth %d", maxFetchDepth)
	}
	if d.kitTransport == nil {
		return nil, fmt.Errorf("skgo: native Kit fetch transport is unavailable")
	}
	request = request.Clone(ctx)
	request.RequestURI = ""
	// Match Kit's request boundary: explicit incoming context overrides the
	// caller's active context, using the application's configured propagator.
	propagator := otel.GetTextMapPropagator()
	carrier := propagation.HeaderCarrier(request.Header)
	propagator.Inject(propagator.Extract(ctx, carrier), carrier)
	request.Header.Set("X-Skgo-Internal-Fetch", d.secret)
	request.Header.Set("X-Skgo-Fetch-Depth", strconv.Itoa(depth))
	if source != nil {
		if address, err := clientAddressOf(source).get(); err == nil {
			request.Header.Set("X-Skgo-Client-Address", address)
		}
	}
	return d.kitTransport.RoundTrip(request)
}

func nativeKitTransport(socket string) *http.Transport {
	if socket == "" {
		return nil
	}
	return &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
}

func (d *NativeDevelopment) isRemoteProtocol(request *http.Request) bool {
	return d.renderer != nil && d.renderer.remotes != nil && strings.HasPrefix(request.URL.Path, d.renderer.remotes.Prefix())
}
