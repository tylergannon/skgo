module github.com/tylergannon/skgo/example

go 1.27.1

// web/ is a Go package (it embeds the build), so without this every ./... walk
// descends into web/node_modules looking for Go packages.
ignore ./web/node_modules

require (
	github.com/tylergannon/polytype v1.0.0
	github.com/tylergannon/skgo v0.0.0
)

require (
	github.com/dave/dst v0.27.4 // indirect
	github.com/dlclark/regexp2/v2 v2.8.0 // indirect
	github.com/dop251/goja v0.0.0-20260911104922-fabc3b8078ad // indirect
	github.com/dop251/goja_nodejs v0.0.0-20260212111938-1f56ff5bcf14 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20260906184651-6331bc6350fe // indirect
	github.com/tylergannon/structtag v0.1.0 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/tools v0.50.0 // indirect
)

replace github.com/tylergannon/skgo => ../

tool github.com/tylergannon/skgo/cmd/skgo
