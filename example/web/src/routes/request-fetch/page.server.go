package requestfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tylergannon/skgo"
)

// PageData is what the load learned by fetching the app's own endpoint.
type PageData struct {
	Fact    string `json:"fact"`
	Visitor string `json:"visitor"`
}

// pageLoad calls /api/request-fetch the way a load calls any service: with
// Event.Fetch, a relative URL, and no socket. The visitor's session cookie
// travels with the subrequest, so the endpoint knows who is asking.
func pageLoad(ctx context.Context) (PageData, error) {
	request, err := http.NewRequest(http.MethodGet, "/api/request-fetch", nil)
	if err != nil {
		return PageData{}, err
	}
	response, err := skgo.EventFrom(ctx).Fetch(ctx, request)
	if err != nil {
		return PageData{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return PageData{}, skgo.Errorf(502, "the request-fetch service answered %d", response.StatusCode)
	}
	var data PageData
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		return PageData{}, fmt.Errorf("reading the request-fetch service: %w", err)
	}
	return data, nil
}

var _ = skgo.Load(pageLoad)
