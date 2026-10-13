package generate_test

import (
	"fmt"
	"github.com/tylergannon/skgo/generate"
)

func ExamplePrepare() {
	prepared, err := generate.Prepare(generate.Config{
		Web: "web", Out: "generated", LocalsPackage: "example.com/app/internal/app",
	})
	if err != nil {
		panic(err)
	}
	defer prepared.Close()
	// An embedding generator can inspect these same ASTs and go/types objects
	// instead of independently loading the application's remote packages.
	for _, pkg := range prepared.Packages() {
		fmt.Println(pkg.PkgPath, len(pkg.Syntax))
	}
	if err := prepared.Run(); err != nil {
		panic(err)
	}
}
