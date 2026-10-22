module github.com/tylergannon/skgo/plugins/cli/example

go 1.27.2

ignore (
	./web/node_modules
	./web/src/routes
)

require (
	github.com/mark3labs/mcp-go v1.3.0
	github.com/spf13/cobra v1.10.2
	github.com/tylergannon/devalue/v5 v5.0.0
	github.com/tylergannon/skgo v0.43.0
	github.com/tylergannon/skgo/plugins/cli v0.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dave/dst v0.28.0 // indirect
	github.com/dlclark/regexp2/v2 v2.9.0 // indirect
	github.com/dop251/goja v0.0.0-20261016120441-313b6b662e53 // indirect
	github.com/dop251/goja_nodejs v0.0.0-20260918173711-b481721df8a2 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/jsonschema-go v0.4.2 // indirect
	github.com/google/pprof v0.0.0-20261016150608-1facf8f91b50 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/tylergannon/polytype v1.10.0 // indirect
	github.com/tylergannon/structtag v0.1.0 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/term v0.47.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/tylergannon/skgo => ../../..

replace github.com/tylergannon/skgo/plugins/cli => ..

tool github.com/tylergannon/skgo/cmd/skgo
