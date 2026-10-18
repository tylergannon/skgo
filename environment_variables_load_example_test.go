package skgo_test

import (
	"fmt"
	"testing/fstest"

	"github.com/tylergannon/skgo"
)

// LoadEnvironment validates the dynamic variables when the server starts and
// returns the typed struct and the snapshot the renderer needs. A missing or
// malformed variable is an error that names the variable and never prints its
// value, so a server stops with log.Fatal(err) before it serves anything.
// In a server, dist is the embedded build NewHandler receives and lookup is
// os.LookupEnv.
func ExampleLoadEnvironment() {
	type Config struct {
		DatabaseURL string `env:"DATABASE_URL"`
		Workers     int    `env:"WORKERS"`
	}
	dist := fstest.MapFS{"env.json": {Data: []byte(`{"fields": [
		{"name": "DATABASE_URL", "field": "DatabaseURL", "type": "string"},
		{"name": "WORKERS", "field": "Workers", "type": "int"}
	]}`)}}

	for _, workers := range []string{"4", "four"} {
		lookup := func(name string) (string, bool) {
			value, ok := map[string]string{"DATABASE_URL": "postgres://db.internal/app", "WORKERS": workers}[name]
			return value, ok
		}
		config, environment, err := skgo.LoadEnvironment[Config](dist, lookup)
		if err != nil {
			fmt.Println(err)
			continue
		}
		// The snapshot goes to the renderer; config goes to your own code.
		_ = skgo.SSROptions{Environment: environment}
		fmt.Println("workers:", config.Workers)
	}
	// Output:
	// workers: 4
	// skgo: environment WORKERS must be int
}
