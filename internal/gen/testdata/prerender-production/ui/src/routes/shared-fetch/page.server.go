package sharedfetch

import (
	"github.com/tylergannon/skgo"
	"io"
	"net/http"
)

type Data struct {
	GoBody   string `json:"goBody"`
	GoHeader string `json:"goHeader"`
	Locals   string `json:"locals"`
}

func load(event PageRequestEvent) (Data, error) {
	request, _ := http.NewRequest("GET", "/shared-alias/nested", nil)
	request.Header.Set("X-Fixture-Go", "yes")
	response, err := event.Fetch(event.Context(), request)
	if err != nil {
		return Data{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return Data{GoBody: string(body), GoHeader: response.Header.Get("X-Config-Hook"), Locals: event.Locals.Value}, err
}

var _ = skgo.Load(load)
