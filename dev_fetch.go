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
	"net/url"
	"sync"
	"sync/atomic"
)

// A fetch exchange suspends only the Go hook's supplied continuation. Kit
// performs that continuation using its live router, credentials and cookies.
type devFetchExchange struct {
	ctx       context.Context
	cancel    context.CancelFunc
	next      chan devFetchContinuation
	mu        sync.Mutex
	reply     chan *http.Response
	continued chan struct{}
	result    chan devFetchResult
	metadata  map[*http.Request]devFetchMetadata
}
type devFetchContinuation struct {
	request *http.Request
	reply   chan *http.Response
}
type devFetchMetadata struct {
	URL        string `json:"url"`
	Redirected bool   `json:"redirected"`
	Type       string `json:"type"`
}
type devFetchReply struct {
	prerenderResponse
	devFetchMetadata
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
		exchange = &devFetchExchange{ctx: ctx, cancel: cancel, next: make(chan devFetchContinuation), result: make(chan devFetchResult, 1), metadata: map[*http.Request]devFetchMetadata{}}
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
		// Body consumption is optional. The logical owner, rather than EOF in
		// a Web Response, is the final lifetime boundary for every exchange.
		entry.active.Add(1)
		context.AfterFunc(ctx, func() { defer entry.active.Done(); exchanges.remove(id, exchange) })
		go func() {
			defer entry.active.Done()
			next := func(request *http.Request) (*http.Response, error) {
				reply := make(chan *http.Response)
				select {
				case exchange.next <- devFetchContinuation{request, reply}:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				select {
				case response := <-reply:
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
		if operation == "/fetch-next" {
			exchange.mu.Lock()
			accepted := exchange.continued
			exchange.mu.Unlock()
			if accepted == nil {
				http.Error(w, "no pending development fetch", 409)
				return
			}
			select {
			case <-accepted:
			case <-exchange.ctx.Done():
				http.Error(w, "request owner ended", 409)
				return
			case <-r.Context().Done():
				exchange.cancel()
				return
			}
		} else {
			raw, err := base64.StdEncoding.DecodeString(r.Header.Get("X-Skgo-Response"))
			var metadata devFetchReply
			if err != nil || json.Unmarshal(raw, &metadata) != nil || metadata.Status < 200 || metadata.Status > 599 {
				http.Error(w, "invalid fetch response", 400)
				return
			}
			headers := http.Header{}
			for name, values := range metadata.Headers {
				headers[http.CanonicalHeaderKey(name)] = append([]string(nil), values...)
			}
			body := &devFetchBody{ReadCloser: r.Body, done: make(chan struct{})}
			response := &http.Response{StatusCode: metadata.Status, Header: headers, Body: body}
			if metadata.URL != "" {
				finalURL, err := url.Parse(metadata.URL)
				if err != nil {
					http.Error(w, "invalid fetch response URL", 400)
					return
				}
				response.Request = &http.Request{URL: finalURL}
				exchange.mu.Lock()
				exchange.metadata[response.Request] = metadata.devFetchMetadata
				exchange.mu.Unlock()
			}
			exchange.mu.Lock()
			reply := exchange.reply
			accepted := exchange.continued
			exchange.reply = nil
			exchange.mu.Unlock()
			if reply == nil {
				http.Error(w, "no pending development fetch", 409)
				return
			}
			select {
			case reply <- response:
				close(accepted)
			case <-exchange.ctx.Done():
				http.Error(w, "request owner ended", 409)
				return
			case <-r.Context().Done():
				exchange.cancel()
				return
			}
			// net/http owns the uploaded body until this handler returns. The
			// separate /fetch-next exchange can advance while Go reads it.
			select {
			case <-body.done:
			case <-exchange.ctx.Done():
			case <-r.Context().Done():
				exchange.cancel()
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	w.Header().Set("X-Skgo-Fetch", id)
	select {
	case continuation := <-exchange.next:
		exchange.mu.Lock()
		exchange.reply = continuation.reply
		exchange.continued = make(chan struct{})
		exchange.mu.Unlock()
		request := continuation.request
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
		w.Header().Set("Content-Type", "application/json")
		w.Write(raw)
	case result := <-exchange.result:
		if result.err != nil {
			http.Error(w, result.err.Error(), 500)
			return
		}
		if result.response.Body == nil {
			result.response.Body = http.NoBody
		}
		defer result.response.Body.Close()
		for name, values := range result.response.Header {
			w.Header()[name] = values
		}
		exchange.mu.Lock()
		metadata, hasMetadata := exchange.metadata[result.response.Request]
		exchange.mu.Unlock()
		if hasMetadata {
			metadata.URL = result.response.Request.URL.String()
			raw, _ := json.Marshal(metadata)
			w.Header().Set("X-Skgo-Fetch-Metadata", base64.StdEncoding.EncodeToString(raw))
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

func (s *devFetchRequests) remove(id string, exchange *devFetchExchange) {
	s.mu.Lock()
	if s.entries[id] == exchange {
		delete(s.entries, id)
	}
	s.mu.Unlock()
}

// Releasing a continuation response body releases its upload handler. A body
// may be streamed, ignored until later, or closed without being read.
type devFetchBody struct {
	io.ReadCloser
	done   chan struct{}
	once   sync.Once
	closed atomic.Bool
}

func (b *devFetchBody) Read(p []byte) (int, error) {
	if b.closed.Load() {
		return 0, http.ErrBodyReadAfterClose
	}
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.once.Do(func() { close(b.done) })
	}
	return n, err
}
func (b *devFetchBody) Close() error {
	// Closing net/http's server body here can drain an unfinished producer.
	// Release its handler instead; the acknowledgment cancels the JS upload,
	// and net/http remains responsible for closing its own request body.
	b.closed.Store(true)
	b.once.Do(func() { close(b.done) })
	return nil
}
