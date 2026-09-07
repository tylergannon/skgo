module github.com/tylergannon/skgo/example

go 1.27.1

require (
	github.com/tylergannon/polytype v1.0.0-rc.11
	github.com/tylergannon/skgo v0.0.0
)

require (
	github.com/dave/dst v0.27.3 // indirect
	github.com/dlclark/regexp2/v2 v2.5.2 // indirect
	github.com/dop251/goja v0.0.0-20260906210903-70ad66ec7ce4 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/google/pprof v0.0.0-20230207041349-798e818bf904 // indirect
	github.com/tylergannon/structtag v0.1.0 // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
)

replace github.com/tylergannon/skgo => ../

tool github.com/tylergannon/skgo/cmd/skgo
