package main

import (
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"

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
	address := os.Getenv("LISTEN_ADDRESS")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("http://" + listener.Addr().String())
	log.Fatal(http.Serve(listener, handler))
}
