package main

/*
#include <stdlib.h>
*/
import "C"

import (
	app "github.com/tylergannon/skgo/plugins/native-shell/example"
	nativehost "github.com/tylergannon/skgo/plugins/native-shell/example/native/loopback"
	"github.com/tylergannon/skgo/plugins/native-shell/example/web"
	"io/fs"
	"log"
	"net/http"
)

var host = nativehost.New(func(origin string) (http.Handler, error) {
	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		return nil, err
	}
	handler, err := app.NewHandler(dist, origin)
	return handler, err
})

//export SKGoHostStart
func SKGoHostStart() *C.char {
	origin, err := host.Start()
	if err != nil {
		log.Print(err)
		return nil
	}
	return C.CString(origin)
}

//export SKGoHostRecover
func SKGoHostRecover() C.int {
	if err := host.Recover(); err != nil {
		log.Print(err)
		return 0
	}
	return 1
}

//export SKGoHostStop
func SKGoHostStop() {
	if err := host.Stop(); err != nil {
		log.Print(err)
	}
}
func main() {}
