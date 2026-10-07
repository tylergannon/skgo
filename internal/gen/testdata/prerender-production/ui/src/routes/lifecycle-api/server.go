package lifecycleapi

import (
	"fmt"
	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/internal/app"
	"net/http"
)

func get(w http.ResponseWriter, r *http.Request) {
	locals := app.LocalsFrom(r.Context())
	if locals.Value != "/lifecycle-api" || locals.Calls != 10 || !skgo.EventFrom(r.Context()).IsSubRequest() {
		http.Error(w, "fetch isolation broken", 500)
		return
	}
	fmt.Fprint(w, "Go fetch: own locals 10")
}

var _ = skgo.GET(get)
