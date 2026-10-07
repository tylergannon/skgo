package skgo

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
)

// Request metadata is independent of caller capabilities. Remote queries keep
// the flags while URL and RouteID still refuse reads before reaching this data.
type requestMetadata struct {
	url                     *url.URL
	routeID                 string
	isData, isRemote, isSub bool
	routing                 bool
}
type requestMetadataKey struct{}

func requestMetadataOf(r *http.Request, u *url.URL, routeID string, remote bool) *requestMetadata {
	if metadata, _ := r.Context().Value(requestMetadataKey{}).(*requestMetadata); metadata != nil {
		if metadata.routing || routeID == "" {
			return metadata
		}
		copy := *metadata
		copy.url, copy.routeID = u, routeID
		return &copy
	}
	return &requestMetadata{url: u, routeID: routeID, isData: hasDataSuffix(r.URL.Path), isRemote: remote, isSub: fetchDepthOf(r.Context()) > 0}
}

// IsDataRequest reports a client __data.json request in every request context.
func (e *Event) IsDataRequest() bool { return e != nil && e.metadata != nil && e.metadata.isData }

// IsRemoteRequest reports an incoming remote transport call. Calling a query
// during an ordinary page render does not change the kind of that request.
func (e *Event) IsRemoteRequest() bool { return e != nil && e.metadata != nil && e.metadata.isRemote }

// IsSubRequest reports an in-process fetch request, rather than a direct nested
// function invocation within the same request.
func (e *Event) IsSubRequest() bool { return e != nil && e.metadata != nil && e.metadata.isSub }

type clientAddressKey struct{}
type clientAddressState struct {
	once    sync.Once
	resolve func() (string, error)
	value   string
	err     error
}

func (a *clientAddressState) get() (string, error) {
	a.once.Do(func() { a.value, a.err = a.resolve() })
	return a.value, a.err
}

func clientAddressOf(r *http.Request) *clientAddressState {
	if address, _ := r.Context().Value(clientAddressKey{}).(*clientAddressState); address != nil {
		return address
	}
	return &clientAddressState{resolve: func() (string, error) { return peerAddress(r) }}
}

func withClientAddress(r *http.Request, provider func(*http.Request) (string, error)) *http.Request {
	if address, _ := r.Context().Value(clientAddressKey{}).(*clientAddressState); address != nil {
		return r
	}
	if provider == nil {
		provider = peerAddress
	}
	address := &clientAddressState{resolve: func() (string, error) { return provider(r) }}
	return r.WithContext(context.WithValue(r.Context(), clientAddressKey{}, address))
}

func peerAddress(r *http.Request) (string, error) {
	if r == nil || r.RemoteAddr == "" {
		return "", errors.New("skgo: client address is unavailable in this request context")
	}
	host := r.RemoteAddr
	if net.ParseIP(host) == nil {
		var err error
		host, _, err = net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return "", fmt.Errorf("skgo: invalid client address %q: %w", r.RemoteAddr, err)
		}
	}
	if net.ParseIP(host) == nil {
		return "", fmt.Errorf("skgo: invalid client IP %q", host)
	}
	return host, nil
}

// ClientAddress returns the hosting boundary's visitor address. The provider's
// first value or error is retained for the request, derived events and internal
// fetches. Build-only and requestless contexts have no address and return an error.
func (e *Event) ClientAddress() (string, error) {
	if e == nil || e.req == nil {
		return peerAddress(nil)
	}
	return clientAddressOf(e.req).get()
}
