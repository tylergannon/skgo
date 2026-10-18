package skgo

// ApplicationConfig holds application policy shared by serving and prerendering.
// Keep an exported value in a Go package and select it with generate's
// --config-package and --config-symbol flags. The generated prerender command
// reads that value while retaining ownership of generated registrations.
//
// Read the same value when configuring LoadConfig, RemoteConfig, EndpointConfig,
// HandleConfig, FetchConfig and SSROptions in the application's runtime stack.
// Transport, matchers and request middleware retain their generated ownership.
type ApplicationConfig struct {
	// HandleFetch surrounds server fetches in Go and in Kit universal loads.
	HandleFetch HandleFetch
	// HandleError supplies the public error policy for serving and building.
	HandleError HandleError
	// FilterSerializedResponseHeaders is the renderer's default. An explicit
	// per-request ResolveOptions predicate replaces it in both serving modes.
	FilterSerializedResponseHeaders func(name, value string) bool
}
