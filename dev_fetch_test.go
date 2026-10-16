package skgo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestNativeFetchKeepsGoHookAroundKitContinuation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entry := &prerenderRequest{request: httptest.NewRequest("GET", "http://example.test/page", nil).WithContext(ctx)}
	sessions := &prerenderRequests{entries: map[string]*prerenderRequest{"owner": entry}}
	exchanges := &devFetchRequests{}
	defer exchanges.close()
	d := &NativeDevelopment{renderer: &SSR{handleFetch: func(ctx context.Context, request *http.Request, next Fetch) (*http.Response, error) {
		if got := request.Header.Get("X-Js-Lowercase"); got != "literal-input-660" {
			t.Errorf("lowercase JS request header = %q", got)
		}
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
	body, _ := json.Marshal(devFetchInput{Handle: "owner", URL: "http://example.test/original", Method: "GET", Headers: http.Header{"x-js-lowercase": {"literal-input-660"}}})
	response, err := http.Post(server.URL+"/fetch-begin", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	id := response.Header.Get("X-Skgo-Fetch")
	defer response.Body.Close()
	if response.Header.Get("X-Skgo-Fetch-State") != "next" {
		t.Fatal("hook failed to yield Kit continuation")
	}
	raw, err := io.ReadAll(response.Body)
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

func TestNativeFetchLowercaseLengthIsParsableByNode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entry := &prerenderRequest{request: httptest.NewRequest("GET", "http://app.test/parent", nil).WithContext(ctx)}
	sessions := &prerenderRequests{entries: map[string]*prerenderRequest{"literal-owner": entry}}
	exchanges := &devFetchRequests{}
	defer exchanges.close()
	d := &NativeDevelopment{renderer: &SSR{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { d.fetch(w, r, sessions, exchanges, r.URL.Path) }))
	defer server.Close()
	const program = `
import assert from 'node:assert/strict';
import {request} from 'node:http';
const origin=process.argv[1];
function call(path,body,headers={}) {
 return new Promise((resolve,reject)=>{
  const req=request(origin+path,{method:'POST',headers},response=>{
   let body='';response.setEncoding('utf8');response.on('data',chunk=>body+=chunk);
   response.on('end',()=>resolve({status:response.statusCode,headers:response.headers,body}));
   response.on('error',reject);
  });req.on('error',reject);req.end(body);
 });
}
const begin=await call('/fetch-begin',JSON.stringify({handle:'literal-owner',url:'http://app.test/inner',method:'GET',headers:{}}),{'content-type':'application/json'});
assert.equal(begin.status,200);
const id=begin.headers['x-skgo-fetch'];
// Headers() iteration in native Kit produces these lowercase names. Their
// casing must survive JSON but acquire Go HTTP map semantics at the boundary.
const metadata={status:201,headers:{'content-length':['31'],'content-type':['text/html'],'x-kit-page':['literal-993']}};
const result=await call('/fetch-response','native Kit document literal-664',{'content-type':'application/octet-stream','x-skgo-fetch':id,'x-skgo-response':Buffer.from(JSON.stringify(metadata)).toString('base64')});
assert.equal(result.status,201);
assert.equal(result.headers['content-length'],'31');
assert.equal(result.headers['transfer-encoding'],undefined);
assert.equal(result.headers['x-kit-page'],'literal-993');
assert.equal(result.body,'native Kit document literal-664');
assert.equal((await call('/fetch-end','',{'x-skgo-fetch':id})).status,204);
`
	commandCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	command := exec.CommandContext(commandCtx, "node", "--input-type=module", "-e", program, server.URL)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("lowercase Kit response framing: %v\n%s", err, output)
	}
	entry.active.Wait()
}

func TestNativeFetchOwnerRetiresUnreadAndPendingExchanges(t *testing.T) {
	for _, completed := range []bool{false, true} {
		t.Run(fmt.Sprint(completed), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			close(done)
			entry := &prerenderRequest{request: httptest.NewRequest("GET", "http://app.test/page", nil).WithContext(ctx), cancel: cancel, done: done}
			sessions := &prerenderRequests{entries: map[string]*prerenderRequest{"owner": entry}}
			exchanges := &devFetchRequests{}
			defer exchanges.close()
			development := &NativeDevelopment{renderer: &SSR{}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { development.fetch(w, r, sessions, exchanges, r.URL.Path) }))
			defer server.Close()
			begin, err := http.Post(server.URL+"/fetch-begin", "application/json", strings.NewReader(`{"handle":"owner","url":"http://app.test/check","method":"GET"}`))
			if err != nil {
				t.Fatal(err)
			}
			id := begin.Header.Get("X-Skgo-Fetch")
			io.Copy(io.Discard, begin.Body)
			begin.Body.Close()
			if id == "" {
				t.Fatal("no exchange")
			}
			var unread *http.Response
			if completed {
				response, _ := http.NewRequest("POST", server.URL+"/fetch-response", strings.NewReader("literal-unread-739"))
				response.Header.Set("X-Skgo-Fetch", id)
				response.Header.Set("X-Skgo-Response", base64.StdEncoding.EncodeToString([]byte(`{"status":200,"headers":{}}`)))
				unread, err = server.Client().Do(response)
				if err != nil {
					t.Fatal(err)
				}
				defer unread.Body.Close()
				if unread.StatusCode != 200 {
					t.Fatal(unread.Status)
				}
			}
			// No /fetch-end and no body drain: request ownership must suffice.
			if err := sessions.end("owner"); err != nil {
				t.Fatal(err)
			}
			exchanges.mu.Lock()
			count := len(exchanges.entries)
			exchanges.mu.Unlock()
			if count != 0 {
				t.Fatalf("ended owner retained %d fetches", count)
			}
		})
	}
}
