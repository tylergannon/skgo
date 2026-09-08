module github.com/tylergannon/skgo/example

go 1.27.1

// web/ is a Go package (it embeds the build), so without this every ./... walk
// descends into web/node_modules looking for Go packages.
ignore ./web/node_modules

require (
	github.com/tylergannon/polytype v1.0.0-rc.11
	github.com/tylergannon/skgo v0.0.0
)

require (
	github.com/dave/dst v0.27.3 // indirect
	github.com/dlclark/regexp2/v2 v2.5.2 // indirect
	github.com/dop251/goja v0.0.0-20260906210903-70ad66ec7ce4 // indirect
	github.com/dop251/goja_nodejs v0.0.0-20251015164255-5e94316bedaf // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/google/pprof v0.0.0-20240727154555-813a5fbdbec8 // indirect
	github.com/tylergannon/structtag v0.1.0 // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
)

replace github.com/tylergannon/skgo => ../

tool github.com/tylergannon/skgo/cmd/skgo
