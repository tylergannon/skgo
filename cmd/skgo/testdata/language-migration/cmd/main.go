package main

import (
	"log"
	"net/http"
	"os"

	"example.com/migration"
)

func main() {
	handler, err := migration.NewHandler(os.DirFS("web/build"))
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", handler))
}
