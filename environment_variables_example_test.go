package skgo_test

import (
	"fmt"
	"testing/fstest"

	"github.com/tylergannon/skgo"
)

// Config is the application's environment. In an application this type and
// the marker below it are the whole of src/env.go, and skgo generate writes
// src/env.ts and web/skgo.env.json from them.
type Config struct {
	// DatabaseURL is where the application keeps its data. It has no flags,
	// so it is private and read when the server starts.
	DatabaseURL string `env:"DATABASE_URL"`
	// SiteName is shown on every page, so it is public.
	SiteName string `env:"SITE_NAME,public"`
	// Release names the build. It is static: vp build reads it and bakes it
	// into the client.
	Release string `env:"RELEASE,public,static"`
	// Workers is how many background workers to start.
	Workers int `env:"WORKERS"`
	// Ratio is the fraction of requests to sample.
	Ratio float64 `env:"SAMPLE_RATIO"`
	// Beta turns the beta banner on. A pointer field is optional.
	Beta *bool `env:"BETA,public"`
}

var _ = skgo.Environment[Config]()

// Each exported field of the declared struct is one variable, typed by its Go
// type. Here the build stands in for the embedded build, whose env.json vp
// build wrote from the declaration together with the static RELEASE it read;
// BETA is unset, so the optional field is nil.
func ExampleEnvironment() {
	build := fstest.MapFS{"env.json": {Data: []byte(`{
		"fields": [
			{"name": "DATABASE_URL", "field": "DatabaseURL", "type": "string"},
			{"name": "SITE_NAME", "field": "SiteName", "type": "string", "public": true},
			{"name": "RELEASE", "field": "Release", "type": "string", "public": true, "static": true},
			{"name": "WORKERS", "field": "Workers", "type": "int"},
			{"name": "SAMPLE_RATIO", "field": "Ratio", "type": "float64"},
			{"name": "BETA", "field": "Beta", "type": "bool", "public": true, "optional": true}
		],
		"static": {"RELEASE": "2026.10"}
	}`)}}
	values := map[string]string{
		"DATABASE_URL": "postgres://db.internal/app",
		"SITE_NAME":    "Acme",
		"WORKERS":      "4",
		"SAMPLE_RATIO": "0.25",
	}
	lookup := func(name string) (string, bool) { value, ok := values[name]; return value, ok }

	config, _, err := skgo.LoadEnvironment[Config](build, lookup)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(config.SiteName, config.Release, config.Workers, config.Ratio, config.Beta == nil)
	// Output: Acme 2026.10 4 0.25 true
}
