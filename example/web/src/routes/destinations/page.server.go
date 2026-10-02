// Package destinations is a Go load that fetches one of every kind of
// destination this app has with Event.Fetch: a rendered page, a data route, a
// Go remote function, a static asset, a prerendered page and a prerendered
// remote result, a prerendered page a dynamic route would also match, and a
// server route. Each answers in-process, from the same stack a browser reaches.
package destinations

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/tylergannon/skgo"
)

// Answer is what one fetch came back with, untouched.
type Answer struct {
	Path        string `json:"path"`
	Status      int    `json:"status"`
	ContentType string `json:"contentType"`
	Body        string `json:"body"`
}

// PageData is every answer, in the order the load asked.
type PageData struct {
	Answers []Answer `json:"answers"`
}

var paths = []string{
	"/request-fetch",
	"/request-fetch/__data.json?x-sveltekit-invalidated=01",
	"/_app/remote/4cga8b/whoami",
	"/robots.txt",
	"/prerender/atlas",
	"/_app/remote/ks8sip/buildReceipt/WyJhdGxhcyJd",
	"/shadow/fixed",
	"/shadow/other",
	"/api/request-fetch",
}

func pageLoad(ctx context.Context) (PageData, error) {
	var data PageData
	for _, path := range paths {
		request, err := http.NewRequest(http.MethodGet, path, nil)
		if err != nil {
			return PageData{}, err
		}
		response, err := skgo.EventFrom(ctx).Fetch(ctx, request)
		if err != nil {
			return PageData{}, fmt.Errorf("fetching %s: %w", path, err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			return PageData{}, err
		}
		data.Answers = append(data.Answers, Answer{
			Path: path, Status: response.StatusCode,
			ContentType: response.Header.Get("Content-Type"), Body: string(body),
		})
	}
	return data, nil
}

var _ = skgo.Load(pageLoad)
