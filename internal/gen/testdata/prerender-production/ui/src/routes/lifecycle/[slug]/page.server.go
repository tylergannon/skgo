package lifecyclepage

import (
	"context"
	"fmt"
	"github.com/tylergannon/skgo"
	"io"
	"net/http"
)

type Data struct {
	Page    string                `json:"page"`
	Fetched string                `json:"fetched"`
	Address string                `json:"address"`
	Later   skgo.Deferred[string] `json:"later"`
}

func load(event PageRequestEvent) (Data, error) {
	event.Locals.Calls++
	if _, err := event.ClientAddress(); err == nil {
		return Data{}, fmt.Errorf("build address unexpectedly available")
	}
	request, _ := http.NewRequest("GET", "/lifecycle-api", nil)
	response, err := event.Fetch(event.Context(), request)
	if err != nil {
		return Data{}, err
	}
	defer response.Body.Close()
	fetched, err := io.ReadAll(response.Body)
	if err != nil {
		return Data{}, err
	}
	return Data{Page: fmt.Sprintf("%s:page-%d", event.Locals.Value, event.Locals.Calls), Fetched: string(fetched), Address: "address unavailable", Later: skgo.Async(event.Context(), func(_ context.Context) (string, error) { return "deferred:" + event.Locals.Value, nil })}, nil
}

var _ = skgo.Load(load)
