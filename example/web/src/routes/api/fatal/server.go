// Package fatalapi demonstrates the response a Go endpoint gives when its
// handler cannot finish. The route deliberately panics before writing bytes.
package fatalapi

import (
	"net/http"

	"github.com/tylergannon/skgo"
)

func fail(http.ResponseWriter, *http.Request) {
	panic("private endpoint failure")
}

var _ = skgo.GET(fail)
