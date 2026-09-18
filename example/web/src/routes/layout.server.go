package site

import (
	"context"

	"github.com/tylergannon/skgo"
)

// RootLayoutData is what every page in the app is handed before it renders,
// because the root layout is in every branch.
type RootLayoutData struct {
	// Deployment names the running build. It is the one value on the page that
	// no remote function answers, so a page showing it is a page whose root
	// layout load ran.
	Deployment string `json:"deployment"`
	// ScriptSafe is a hostile hydration value used only by the browser proof.
	// It is empty unless the proof query parameter asks for it.
	ScriptSafe string `json:"scriptSafe"`
}

const scriptSafeFixture = `</script><script>globalThis.__skgo_injected=true</script>&<>`

// layoutLoad is the app's outermost load, and the one place a failure has
// nowhere to go.
//
// Kit wraps the root error page *inside* the root layout rather than the other
// way round (`runtime/error-chain.js`), so no `+error.svelte` is above this
// node: kit's answer to a load that fails here is the static `error.html` the
// build carries — a whole document with no app in it and no script, so nothing
// boots and nothing tries again (`runtime/server/errors.js`, `static_error_page`).
//
// The refusal is on a query parameter rather than on anything ambient, so a
// scenario can ask for it and no other request can stumble into it.
func layoutLoad(ctx context.Context) (RootLayoutData, error) {
	event := skgo.EventFrom(ctx)
	if boom, _ := event.SearchParam("boom"); boom == "root-layout" {
		return RootLayoutData{}, skgo.Errorf(503, "The root layout could not reach the database")
	}
	data := RootLayoutData{Deployment: "skgo example"}
	if proof, _ := event.SearchParam("proof"); proof == "script-safe" {
		data.ScriptSafe = scriptSafeFixture
	}
	return data, nil
}

var _ = skgo.Load(layoutLoad)
