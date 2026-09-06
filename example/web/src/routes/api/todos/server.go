// Package todosapi serves /api/todos: an ordinary Go HTTP endpoint written
// beside the route that serves it, which is what SvelteKit's `+server.ts` is.
//
// Nothing here is a remote function or a load. It is raw HTTP — the status, the
// headers and the body are this handler's, and skgo neither encodes the result
// nor interprets it. That is what makes the route usable by something that is
// not kit's client: `curl`, a mobile app, a webhook.
package todosapi

import (
	"encoding/json"
	"net/http"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// list answers `GET /api/todos` with the todos this visitor may see.
//
// Who the visitor is comes from the app's Handle hook, exactly as it does in a
// load or a remote function: the endpoint reads the session it stored rather
// than parsing the cookie again.
func list(w http.ResponseWriter, r *http.Request) {
	session, _ := skgo.LocalOf[businesslogic.Session](r.Context())
	writeJSON(w, http.StatusOK, businesslogic.Default.Todos(session.User != ""))
}

// newTodo is the body `POST /api/todos` accepts.
type newTodo struct {
	Text string `json:"text"`
}

// add answers `POST /api/todos`, and answers it the way an HTTP API does: 201,
// a Location header naming what was created, and the created record as the
// body.
func add(w http.ResponseWriter, r *http.Request) {
	var body newTodo
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "expected a JSON object with a text property"})
		return
	}
	if body.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "text must not be empty"})
		return
	}

	todo := businesslogic.Default.Add(body.Text)
	w.Header().Set("Location", "/api/todos/"+todo.ID)
	writeJSON(w, http.StatusCreated, todo)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

var (
	_ = skgo.GET(list)
	_ = skgo.POST(add)
)
