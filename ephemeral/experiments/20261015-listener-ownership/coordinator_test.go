package listenerownership

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// This tests the third topology's lifecycle property, not Kit or Vite behavior.
func TestCoordinatorRetainsPublicListenerAcrossAppReplacement(t *testing.T) {
	var upstream atomic.Pointer[url.URL]
	app := func(value string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "APP_%s host=%s origin=%s", value, r.Host, r.Header.Get("Origin"))
		}))
	}
	first := app("731")
	defer first.Close()
	u, _ := url.Parse(first.URL)
	upstream.Store(u)
	var toolConnections atomic.Int32
	tools := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" {
			io.WriteString(w, "TOOLS_419")
			return
		}
		c, b, e := w.(http.Hijacker).Hijack()
		if e != nil {
			return
		}
		defer c.Close()
		toolConnections.Add(1)
		fmt.Fprint(b, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		b.Flush()
		for {
			s, e := b.ReadString('\n')
			if e != nil {
				return
			}
			fmt.Fprint(b, s)
			b.Flush()
		}
	}))
	defer tools.Close()
	toolURL, _ := url.Parse(tools.URL)
	toolProxy := httputil.NewSingleHostReverseProxy(toolURL)
	proxy := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) { r.SetURL(upstream.Load()); r.Out.Host = r.In.Host }, ErrorHandler: func(w http.ResponseWriter, r *http.Request, e error) { http.Error(w, "GO_APP_UNAVAILABLE", 503) }}
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/@") {
			toolProxy.ServeHTTP(w, r)
		} else {
			proxy.ServeHTTP(w, r)
		}
	}))
	defer edge.Close()
	get := func(wantStatus int, want string) {
		t.Helper()
		req, _ := http.NewRequest("GET", edge.URL+"/", nil)
		req.Header.Set("Origin", edge.URL)
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode != wantStatus || !strings.Contains(string(body), want) {
			t.Fatalf("status=%d body=%s want=%s", res.StatusCode, body, want)
		}
		t.Logf("public %s -> %d %s", edge.URL, res.StatusCode, body)
	}
	get(200, "APP_731 host="+strings.TrimPrefix(edge.URL, "http://")+" origin="+edge.URL)
	c, e := net.Dial("tcp", strings.TrimPrefix(edge.URL, "http://"))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	fmt.Fprintf(c, "GET /@hmr?token=fixture HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Protocol: vite-hmr\r\n\r\n", strings.TrimPrefix(edge.URL, "http://"))
	reader := bufio.NewReader(c)
	response, e := http.ReadResponse(reader, nil)
	if e != nil || response.StatusCode != 101 {
		t.Fatalf("upgrade: %v %v", response, e)
	}
	echo := func(value string) {
		t.Helper()
		fmt.Fprintln(c, value)
		got, e := reader.ReadString('\n')
		if e != nil || got != value+"\n" {
			t.Fatalf("echo=%q err=%v", got, e)
		}
	}
	echo("TOOL_SOCKET_BEFORE_983")
	first.Close()
	get(503, "GO_APP_UNAVAILABLE")
	echo("TOOL_SOCKET_DURING_601")
	second := app("887")
	defer second.Close()
	next, _ := url.Parse(second.URL)
	upstream.Store(next)
	get(200, "APP_887 host="+strings.TrimPrefix(edge.URL, "http://")+" origin="+edge.URL)
	echo("TOOL_SOCKET_AFTER_449")
	if toolConnections.Load() != 1 {
		t.Fatalf("tool reconnect count=%d", toolConnections.Load())
	}
	t.Log("same public origin and same upgraded tool connection survived app absence and replacement on a different private port")
}
