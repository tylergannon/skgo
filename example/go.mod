module github.com/tylergannon/skgo/example

go 1.27.1

require (
	github.com/tylergannon/polytype v1.0.0-rc.10
	github.com/tylergannon/skgo v0.0.0
)

require (
	github.com/dave/dst v0.27.3 // indirect
	github.com/tylergannon/structtag v0.1.0 // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
)

replace github.com/tylergannon/skgo => ../

tool (
	github.com/tylergannon/polytype/polytype
	github.com/tylergannon/skgo/cmd/skgo
)
