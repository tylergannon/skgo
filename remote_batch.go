package skgo

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/tylergannon/skgo/internal/remotearg"
)

// serveBatch answers a `query.batch` call. Kit's client collects every call
// made to one batch query in a macrotask and sends them as a single POST
// carrying the payloads it collected
// (`runtime/client/remote-functions/query-batch.svelte.js`), so the app's Go
// function is invoked once however many components asked.
//
// The answer is kit's ordinary remote-function envelope with `_` holding one
// node per payload, in the order they arrived — `{"type":"result"}` with the
// value, or `{"type":"error"}` with the error the client rejects that one
// entry's promise with. Nothing else is added: a batch query's values reach
// the client through `_` and never through `q`, because the client resolves
// the promises it is already holding rather than seeding a cache.
func (rs *Remotes) serveBatch(w http.ResponseWriter, r *http.Request, fn *Remote) {
	if r.Method != http.MethodPost {
		rs.writeErrorStatus(w, &HTTPError{
			Status:  405,
			Message: "`query.batch` functions must be invoked via POST request, not " + r.Method,
		}, http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Payloads []string `json:"payloads"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		rs.writeError(w, &HTTPError{Status: 400, Message: "Bad Request"})
		return
	}

	calls, err := rs.parsePayloads(body.Payloads)
	if err != nil {
		rs.writeError(w, &HTTPError{Status: 400, Message: "Bad Request"})
		return
	}

	// A batch query is a query: it may read cookies and never write them.
	ev := rs.newEvent(r, false)

	values, err := rs.callBatch(withEvent(r.Context(), ev), fn, calls)
	if err != nil {
		if redirect := asRedirect(err); redirect != nil {
			rs.writeResult(w, ev, map[string]any{"redirect": redirect.Location})
			return
		}
		rs.writeError(w, asHTTPError(err))
		return
	}

	nodes := make([]any, len(values))
	for i, value := range values {
		nodes[i] = map[string]any{"type": "result", "data": value}
	}
	rs.writeResult(w, ev, map[string]any{"_": nodes})
}

// parsePayloads turns every payload in a batch into the call the generated
// closure is handed, keeping the "was there an argument at all" flag each one
// carries.
func (rs *Remotes) parsePayloads(payloads []string) ([]Call, error) {
	calls := make([]Call, len(payloads))
	for i, payload := range payloads {
		arg, ok, err := remotearg.ParsePayloadWith(payload, rs.codecs())
		if err != nil {
			return nil, err
		}
		calls[i] = rs.newCall(arg, ok)
	}
	return calls, nil
}

// callBatch runs a batch query's function, turning a panic into the error
// every path here already knows how to answer. See Remotes.call.
func (rs *Remotes) callBatch(ctx context.Context, fn *Remote, calls []Call) (out []any, err error) {
	defer func() { err = rs.recovered(fn, recover(), err) }()

	values, err := fn.batch(ctx, calls)
	if err != nil {
		// A length disagreement between the arguments and the results is the
		// app's bug and says so on the server; the client gets kit's opaque
		// 500 from asHTTPError like any other unexpected failure.
		if _, expected := err.(*HTTPError); !expected && asRedirect(err) == nil {
			log.Printf("skgo: batch query %s failed: %v", fn.id, err)
		}
		return nil, err
	}

	// The generated closure encoded each value with the codec polytype emitted
	// for the function's own result type, so these are already the trees
	// devalue serializes.
	return values, nil
}
