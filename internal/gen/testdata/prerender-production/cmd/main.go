package main

import (
	"io/fs"
	"log"
	"net/http"

	"github.com/tylergannon/skgo/example"
	"github.com/tylergannon/skgo/example/ui"
)

func main() {
	dist, err := fs.Sub(ui.Build, "build")
	if err != nil {
		log.Fatal(err)
	}
	handler, err := example.NewHandler(dist, nil)
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", handler))
}
