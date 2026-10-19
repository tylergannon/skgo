package skgo_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/tylergannon/skgo"
)

// flyClientAddress reads the client address from the header Fly's proxy sets.
// Install it only when every request arrives through that proxy.
func flyClientAddress(r *http.Request) (string, error) {
	if ip := r.Header.Get("Fly-Client-IP"); ip != "" {
		return ip, nil
	}
	return "", errors.New("no Fly-Client-IP header")
}

// Behind a proxy, HandleConfig.ClientAddress decides what
// event.ClientAddress() returns. Here the handle hook reads it.
func ExampleHandleConfig_behindProxy() {
	handleCfg := skgo.HandleConfig{ClientAddress: flyClientAddress}

	handle := skgo.Handle(func(ctx context.Context) error {
		address, err := skgo.EventFrom(ctx).ClientAddress()
		fmt.Println(address, err)
		return nil
	})
	app := handle.Intercept(handleCfg, http.NotFoundHandler())

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Fly-Client-IP", "203.0.113.7")
	app.ServeHTTP(httptest.NewRecorder(), request)
	// Output: 203.0.113.7 <nil>
}
