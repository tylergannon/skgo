package skgo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNativeFetchKeepsGoHookAroundKitContinuation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entry := &prerenderRequest{request: httptest.NewRequest("GET", "http://example.test/page", nil).WithContext(ctx)}
	sessions := &prerenderRequests{entries: map[string]*prerenderRequest{"owner": entry}}
	exchanges := &devFetchRequests{}
	defer exchanges.close()
	d := &NativeDevelopment{renderer: &SSR{handleFetch: func(ctx context.Context, request *http.Request, next Fetch) (*http.Response, error) {
		request.Header.Set("X-Go-Before", "literal-771")
		request.URL.Path = "/native-kit-page"
		response, err := next(request)
		if err != nil {
			return nil, err
		}
		response.Header.Set("X-Go-After", "literal-882")
		return response, nil
	}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { d.fetch(w, r, sessions, exchanges, r.URL.Path) }))
	defer server.Close()
	body, _ := json.Marshal(devFetchInput{Handle: "owner", URL: "http://example.test/original", Method: "GET", Headers: http.Header{}})
	response, err := http.Post(server.URL+"/fetch-begin", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	id := response.Header.Get("X-Skgo-Fetch")
	response.Body.Close()
	if response.Header.Get("X-Skgo-Fetch-State") != "next" {
		t.Fatal("hook failed to yield Kit continuation")
	}
	raw, err := base64.StdEncoding.DecodeString(response.Header.Get("X-Skgo-Fetch-Request"))
	if err != nil {
		t.Fatal(err)
	}
	var request devFetchInput
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	if request.URL != "http://example.test/native-kit-page" || request.Headers.Get("X-Go-Before") != "literal-771" {
		t.Fatalf("Go hook request: %+v", request)
	}
	metadata, _ := json.Marshal(prerenderResponse{Status: 201, Headers: http.Header{"X-Kit-Page": {"literal-993"}}})
	callback, _ := http.NewRequest("POST", server.URL+"/fetch-response", strings.NewReader("native Kit document literal-664"))
	callback.Header.Set("X-Skgo-Fetch", id)
	callback.Header.Set("X-Skgo-Response", base64.StdEncoding.EncodeToString(metadata))
	final, err := http.DefaultClient.Do(callback)
	if err != nil {
		t.Fatal(err)
	}
	result, err := io.ReadAll(final.Body)
	final.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if final.StatusCode != 201 || final.Header.Get("X-Go-After") != "literal-882" || final.Header.Get("X-Kit-Page") != "literal-993" || string(result) != "native Kit document literal-664" {
		t.Fatalf("hook-wrapped Kit response %d %v %s", final.StatusCode, final.Header, result)
	}
	end, _ := http.NewRequest("POST", server.URL+"/fetch-end", nil)
	end.Header.Set("X-Skgo-Fetch", id)
	ended, err := http.DefaultClient.Do(end)
	if err != nil {
		t.Fatal(err)
	}
	ended.Body.Close()
	entry.active.Wait()
}

func TestNativeFetchRejectsUnknownRequestOwner(t *testing.T) {
	d := &NativeDevelopment{}
	recorder := httptest.NewRecorder()
	d.fetch(recorder, httptest.NewRequest("POST", "/fetch-begin", strings.NewReader(`{"handle":"old-process","url":"http://example.test/","method":"GET"}`)), &prerenderRequests{entries: map[string]*prerenderRequest{}}, &devFetchRequests{}, "/fetch-begin")
	if recorder.Code != 409 {
		t.Fatalf("status=%d", recorder.Code)
	}
}
