package hookrefused

import (
	"os"

	"github.com/tylergannon/skgo"
)

type Data struct {
	Result string `json:"result"`
}

func load(event PageRequestEvent) (Data, error) {
	if event.IsSubRequest() {
		if marker := os.Getenv("SKGO_FIXTURE_REFUSED_LOAD_MARKER"); marker != "" {
			if err := os.WriteFile(marker, []byte("refused load ran"), 0o600); err != nil {
				return Data{}, err
			}
		}
	}
	return Data{Result: "Go load ran"}, nil
}

var _ = skgo.Load(load)
