package skgo

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// A fetch exchange suspends only the Go hook's supplied continuation. Kit
// performs that continuation using its live router, credentials and cookies.
type devFetchExchange struct {
	ctx    context.Context
	cancel context.CancelFunc
	next   chan *http.Request
	reply  chan *http.Response
	result chan devFetchResult
}
type devFetchResult struct {
	response *http.Response
	err      error
}
type devFetchRequests struct {
	mu      sync.Mutex
	entries map[string]*devFetchExchange
}
type devFetchInput struct {
	Handle  string      `json:"handle"`
	URL     string      `json:"url"`
	Method  string      `json:"method"`
	Headers http.Header `json:"headers"`
	Body    []byte      `json:"body"`
}

func (d *NativeDevelopment) fetch(w http.ResponseWriter, r *http.Request, sessions *prerenderRequests, exchanges *devFetchRequests, operation string) {
	_ = http.NewResponseController(w).EnableFullDuplex()
	id := r.Header.Get("X-Skgo-Fetch")
	var exchange *devFetchExchange
	if operation == "/fetch-begin" {
		var input devFetchInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		entry, err := sessions.acquire(input.Handle)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		ctx, cancel := context.WithCancel(entry.request.Context())
		request, err := http.NewRequestWithContext(ctx, input.Method, input.URL, bytes.NewReader(input.Body))
		if err != nil {
			entry.active.Done()
			cancel()
			http.Error(w, err.Error(), 400)
			return
		}
		request.Header = http.Header{}
		for name, values := range input.Headers {
			request.Header[http.CanonicalHeaderKey(name)] = append([]string(nil), values...)
		}
		exchange = &devFetchExchange{ctx: ctx, cancel: cancel, next: make(chan *http.Request), reply: make(chan *http.Response), result: make(chan devFetchResult, 1)}
		token := make([]byte, 24)
		if _, err := rand.Read(token); err != nil {
			entry.active.Done()
			cancel()
			http.Error(w, err.Error(), 500)
			return
		}
		id = hex.EncodeToString(token)
		exchanges.mu.Lock()
		if exchanges.entries == nil {
			exchanges.entries = map[string]*devFetchExchange{}
		}
		exchanges.entries[id] = exchange
		exchanges.mu.Unlock()
		go func() {
			defer entry.active.Done()
			next := func(request *http.Request) (*http.Response, error) {
				select {
				case exchange.next <- request:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				select {
				case response := <-exchange.reply:
					return response, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			hook := d.renderer.handleFetch
			var response *http.Response
			var callErr error
			if hook == nil {
				response, callErr = next(request)
			} else {
				response, callErr = hook(ctx, request, next)
			}
			if response == nil && callErr == nil {
				callErr = fmt.Errorf("skgo: HandleFetch returned no response")
			}
			exchange.result <- devFetchResult{response, callErr}
		}()
	} else {
		exchanges.mu.Lock()
		exchange = exchanges.entries[id]
		exchanges.mu.Unlock()
		if exchange == nil {
			http.Error(w, "unknown development fetch", 409)
			return
		}
		if operation == "/fetch-end" {
			exchanges.mu.Lock()
			delete(exchanges.entries, id)
			exchanges.mu.Unlock()
			exchange.cancel()
			w.WriteHeader(204)
			return
		}
		raw, err := base64.StdEncoding.DecodeString(r.Header.Get("X-Skgo-Response"))
		var metadata prerenderResponse
		if err != nil || json.Unmarshal(raw, &metadata) != nil || metadata.Status < 200 || metadata.Status > 599 {
			http.Error(w, "invalid fetch response", 400)
			return
		}
		headers := http.Header{}
		for name, values := range metadata.Headers {
			headers[http.CanonicalHeaderKey(name)] = append([]string(nil), values...)
		}
		response := &http.Response{StatusCode: metadata.Status, Header: headers, Body: r.Body}
		select {
		case exchange.reply <- response:
		case <-exchange.ctx.Done():
			http.Error(w, "request owner ended", 409)
			return
		case <-r.Context().Done():
			exchange.cancel()
			return
		}
	}
	w.Header().Set("X-Skgo-Fetch", id)
	select {
	case request := <-exchange.next:
		var body []byte
		if request.Body != nil {
			var err error
			body, err = io.ReadAll(request.Body)
			if err != nil {
				exchange.cancel()
				http.Error(w, err.Error(), 500)
				return
			}
		}
		raw, err := json.Marshal(devFetchInput{URL: request.URL.String(), Method: request.Method, Headers: request.Header, Body: body})
		if err != nil {
			exchange.cancel()
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("X-Skgo-Fetch-State", "next")
		w.Header().Set("X-Skgo-Fetch-Request", base64.StdEncoding.EncodeToString(raw))
		w.WriteHeader(204)
	case result := <-exchange.result:
		if result.err != nil {
			http.Error(w, result.err.Error(), 500)
			return
		}
		defer result.response.Body.Close()
		for name, values := range result.response.Header {
			w.Header()[name] = values
		}
		w.Header().Set("X-Skgo-Fetch-State", "final")
		w.WriteHeader(result.response.StatusCode)
		flush := http.NewResponseController(w)
		_ = flush.Flush()
		buf := make([]byte, 32*1024)
		for {
			n, err := result.response.Body.Read(buf)
			if n > 0 {
				if _, e := w.Write(buf[:n]); e != nil {
					exchange.cancel()
					return
				}
				_ = flush.Flush()
			}
			if err != nil {
				if err != io.EOF {
					panic(http.ErrAbortHandler)
				}
				return
			}
		}
	case <-exchange.ctx.Done():
		http.Error(w, "request owner ended", 409)
	case <-r.Context().Done():
		exchange.cancel()
	}
}

func (s *devFetchRequests) close() {
	s.mu.Lock()
	entries := s.entries
	s.entries = map[string]*devFetchExchange{}
	s.mu.Unlock()
	for _, entry := range entries {
		entry.cancel()
	}
}
