package fetchcapabilities

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tylergannon/skgo"
)

type Data struct {
	Page       string `json:"page"`
	Navigation string `json:"navigation"`
	Remote     string `json:"remote"`
	Missing    string `json:"missing"`
	Endpoint   string `json:"endpoint"`
}

func load(event PageRequestEvent) (Data, error) {
	fetch := func(path string) (string, error) {
		request, _ := http.NewRequest("GET", path, nil)
		response, err := event.Fetch(event.Context(), request)
		if err != nil {
			return err.Error(), nil
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		return fmt.Sprintf("response:%d:%s", response.StatusCode, strings.TrimSpace(string(body))), err
	}
	var data Data
	var err error
	if data.Page, err = fetch("/fetch-target"); err != nil {
		return data, err
	}
	if data.Navigation, err = fetch("/fetch-target/__data.json"); err != nil {
		return data, err
	}
	// Existing literal Kit ID of the fixture's noargValue prerender remote.
	if data.Remote, err = fetch("/_app/remote/18787o2/noargValue"); err != nil {
		return data, err
	}
	if data.Missing, err = fetch("/literal-missing-destination-272"); err != nil {
		return data, err
	}
	if data.Endpoint, err = fetch("/lifecycle-api"); err != nil {
		return data, err
	}
	return data, nil
}

var _ = skgo.Load(load)
