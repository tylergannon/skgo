---
title: Environment variables
summary: Declare the environment in Go once; Kit's typed accessors, the build and the server all read that declaration.
proof:
  - TestEnvironmentTypedStartupSnapshot
  - TestEnvironmentGenerationUsesDeclaredGoTypes
  - TestEnvironmentCommandValidatesBuildInputsInGo
  - TestDocumentUsesTypedPublicEnvironmentAndStaticPlaceholders
  - TestEnvironmentEndpointOverridesPrerenderedBuildValues
---

Kit %kit% declares [environment variables](https://svelte.dev/docs/kit/environment-variables) in `src/env.ts` with `defineEnvVars`, and imports them from `$app/env/private` and `$app/env/public`. In SKGo the declaration is a Go struct in `src/env.go`, `skgo generate` writes `src/env.ts` from it, and the same declaration is what the build checks, what the Go server validates at startup, and what your Go code reads its configuration from. There is one list of variables, and it is typed.

## Setup

Declare a struct in `src/env.go` and mark it with `skgo.Environment`:

```go
package env

import "github.com/tylergannon/skgo"

type Config struct {
	DatabaseURL string  `env:"DATABASE_URL"`
	SiteName    string  `env:"SITE_NAME,public"`
	Release     string  `env:"RELEASE,public,static"`
	Workers     int     `env:"WORKERS"`
	Ratio       float64 `env:"SAMPLE_RATIO"`
	Beta        *bool   `env:"BETA,public"`
}

var _ = skgo.Environment[Config]()
```

Each exported field is one variable. The `env` tag names it and takes two flags, `public` and `static`, with the meanings Kit gives them. Fields are `string`, `bool`, `int` or `float64`; a pointer field is optional and is nil when the variable is unset, while every other field is required. Booleans accept `true`, `false`, `yes` and `no` in any case; integers are base ten and must fit JavaScript's safe range; strings keep their exact contents, and an empty string is valid only for a string field. The declaration must be the only one in the application and must live in `src/env.go`.

Run [`skgo generate`](/docs/cli/generate). It writes two files:

- `src/env.ts`, the `defineEnvVars` declaration Kit reads, with one entry per field carrying its `public` and `static` flags and a `schema` that hands Kit the value Go already validated, typed as `string`, `boolean` or `number`, with `| undefined` for an optional field
- `web/skgo.env.json`, the declaration the build and the [`skgo env`](/docs/cli/env) command check values against

A hand-written `src/env.ts` stops generation rather than being overwritten; remove it once the Go declaration exists.

## Private variables

A variable without `public` never leaves the server. It is not in `$app/env/public`, not in the document, and not in the module the browser loads; a test asserts that a private value declared beside public ones is absent from both. Read it in Go, from the struct `LoadEnvironment` returns, as described under [Validation](#Validation).

## Public variables

A variable with `public` can be imported from `$app/env/public` in any component, typed by the generated declaration, and used in `src/app.html` as `%sveltekit.env.SITE_NAME%`, exactly as in Kit. Go renders the document and fills the placeholders, and serves Kit's `_app/env.js` module from the values it validated at startup, so the browser, the document and the server agree.

## Static variables

A variable with `static` binds at build time: `vp build` reads it from the environment of the build, bakes it into `env.json` inside the build output, and Kit inlines it into the client bundle, enabling dead-code elimination as Kit describes. Every other variable is dynamic and is read when the server starts. A static variable is never read from the server's environment, so changing it means rebuilding.

## Validation

Kit validates variables with a schema in `env.ts`; in SKGo the Go types are the schema, and validation happens twice with the same parser.

At build time, the adapter checks the static variables and fails the build when one is missing or malformed, naming the variable.

At startup, `skgo.LoadEnvironment` reads the dynamic variables, validates them against the declaration in the embedded build, and returns the typed struct and a snapshot:

```go
config, environment, err := skgo.LoadEnvironment[env.Config](dist, os.LookupEnv)
if err != nil {
	log.Fatal(err) // names the variable, never prints its value
}
```

`dist` is the embedded build, the same `fs.FS` `NewHandler` receives, and the second argument is where values come from, normally `os.LookupEnv`. Pass the snapshot to the renderer as `skgo.SSROptions.Environment` when `server.go` builds it; a build that declares variables refuses to start without one. Keep `config` for your own code: it is an ordinary Go value, and the right way for a service to learn its database URL is to be handed `config.DatabaseURL`, not to call `os.Getenv`.

Before a deployment, [`skgo env`](/docs/cli/env) runs the same validation from the command line against a JSON object of values, so tooling can refuse a bad environment before the server is started.

## Documenting variables

Unchanged on the Kit side, where `description` in `defineEnvVars` documents a variable; in SKGo the doc comment on the Go field is where the description lives, next to the type that constrains it. See [`skgo: environment`](skgo-environment) for the API.
