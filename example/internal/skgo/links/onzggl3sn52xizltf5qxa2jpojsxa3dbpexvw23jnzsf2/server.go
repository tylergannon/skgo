// Package replayapi holds the responses the /universal-fetch page's universal
// load reads. Each kind is a different shape of answer — text, JSON, bytes that
// are not UTF-8, a body written in chunks, no body at all — so that what Kit
// replays into the document, and what the browser does with it, can be told
// apart. The values are literals; the Gherkin suite and the Go tests state
// them again rather than asking this file.
package replayapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/tylergannon/skgo"
)

// Binary is not valid UTF-8, so no text round trip can carry it.
var Binary = []byte{0x00, 0xff, 0x10, 0x80, 0x7f, 0xc3, 0x28}

const (
	// Allowed is the header the app's filterSerializedResponseHeaders lets
	// into the document.
	Allowed = "shown-7"
	// Secret is the one it does not.
	Secret = "hidden-value-31"
)

func answer(w http.ResponseWriter, r *http.Request) {
	kind := skgo.EventFrom(r.Context()).Param("kind")
	query := r.URL.Query()
	h := w.Header()
	switch kind {
	case "text":
		h.Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("plain-lantern-7"))
	case "json":
		h.Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"lamp": "harbour", "step": query.Get("step")})
	case "binary":
		h.Set("Content-Type", "application/octet-stream")
		h.Set("X-Replay-Allowed", Allowed)
		h.Set("X-Replay-Secret", Secret)
		_, _ = w.Write(Binary)
	case "stream":
		h.Set("Content-Type", "text/plain; charset=utf-8")
		for _, chunk := range []string{"alpha-", "beta-", "gamma"} {
			_, _ = w.Write([]byte(chunk))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	case "breaking":
		h.Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("</script><!-- \u2028 \u2029 \"q\" & ok"))
	case "cookies":
		// Sets two cookies when asked, and says which cookies it was sent.
		if query.Get("set") != "" {
			http.SetCookie(w, &http.Cookie{Name: "lamp", Value: "glow", Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "other", Value: "dim", Path: "/"})
		}
		h.Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(r.Header.Get("Cookie")))
	case "empty":
		h.Set("X-Replay-Allowed", Allowed)
		w.WriteHeader(http.StatusNoContent)
	case "missing":
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("no such lamp"))
	case "cached":
		h.Set("Content-Type", "application/json")
		h.Set("Cache-Control", "max-age=60")
		_, _ = w.Write([]byte(`{"lamp":"cached"}`))
	case "echo", "form":
		body, _ := io.ReadAll(r.Body)
		h.Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"method":      r.Method,
			"header":      r.Header.Get("X-Replay"),
			"contentType": r.Header.Get("Content-Type"),
			"body":        string(body),
			"step":        query.Get("step"),
		})
	case "raw":
		// What a Go endpoint gets from the same capability the universal
		// fetch is built on: no CORS, no replay, just the answer.
		target, err := url.Parse(query.Get("target"))
		if err != nil || target.Scheme != "http" || !strings.HasPrefix(target.Host, "127.0.0.1:") {
			http.Error(w, "target must be a loopback http URL", http.StatusBadRequest)
			return
		}
		request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target.String()+query.Get("path"), nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		response, err := skgo.EventFrom(r.Context()).Fetch(r.Context(), request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		h.Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": response.StatusCode, "body": string(raw)})
	default:
		http.NotFound(w, r)
	}
}

func get(w http.ResponseWriter, r *http.Request)  { answer(w, r) }
func post(w http.ResponseWriter, r *http.Request) { answer(w, r) }

var (
	_ = skgo.GET(get)
	_ = skgo.POST(post)
)
