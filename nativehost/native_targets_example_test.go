package nativehost_test

import (
	"fmt"
	"io/fs"
	"net/http"
	"testing/fstest"

	"github.com/tylergannon/skgo/nativehost"
)

// build stands in for web.Build, the embedded adapter output.
var build fs.FS = fstest.MapFS{"build/skgo.manifest.json": {Data: []byte("{}")}}

// NewHandler stands in for the NewHandler in your module's server.go.
func NewHandler(dist fs.FS, origin string) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "served on the loopback origin")
	}), nil
}

// The native-app add-on's host builds the application's handler through New.
// Start serves it on a loopback origin for the WebView; Recover rebinds that
// origin when the app becomes active again, and Stop drains it.
func ExampleNew() {
	host := nativehost.New(func(origin string) (http.Handler, error) {
		dist, err := fs.Sub(build, "build")
		if err != nil {
			return nil, err
		}
		return NewHandler(dist, origin)
	})
	origin, err := host.Start()
	if err != nil {
		fmt.Println(err)
		return
	}
	defer host.Stop()

	response, err := http.Get(origin + "/")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer response.Body.Close()
	fmt.Println(response.Status)
	// Output: 200 OK
}
