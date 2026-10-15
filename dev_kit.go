package skgo

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
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
