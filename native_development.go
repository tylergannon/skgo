package skgo

import (
	"io/fs"
	"net/http"
)

// NewSSR registers the application's real Go configuration for Kit-owned
// development rendering. Call this explicitly from NewDevelopmentHandler;
// build remains an ordinary filesystem and may be wrapped or composed. A nil
// development receiver creates the ordinary production renderer.
func (d *NativeDevelopment) NewSSR(build fs.FS, m Manifest, loads *Loads, remotes *Remotes, opts SSROptions) (*SSR, error) {
	if d == nil {
		return NewSSR(build, m, loads, remotes, opts)
	}
	s := &SSR{loads: loads, remotes: remotes, actions: opts.Actions, fetch: opts.Fetch, handleFetch: opts.HandleFetch, filterSerializedResponseHeaders: opts.FilterSerializedResponseHeaders, errorPage: "<html><body>%sveltekit.error.message%</body></html>"}
	d.renderer = s
	d.manifest = m
	return s, nil
}

// NewStaticHandler gives Kit the page response while preserving the Go
// application's surrounding request middleware. A nil development receiver
// constructs the normal production static handler with the same options.
func (d *NativeDevelopment) NewStaticHandler(build fs.FS, options ...StaticOption) (http.Handler, error) {
	if d == nil {
		return NewStaticHandler(build, options...)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if next, ok := r.Context().Value(devContinuationKey{}).(http.Handler); ok {
			next.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	}), nil
}
