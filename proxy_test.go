package skgo

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDevProxyForwardsRequest(t *testing.T) {
	var (
		gotMethod string
		gotPath   string
		gotQuery  string
		gotHost   string
		gotBody   string
		gotHeader http.Header
	)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotHost = r.Host
		gotHeader = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)

		w.Header().Set("X-Upstream", "yes")
		w.WriteHeader(http.StatusTeapot)
		io.WriteString(w, "from vite")
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	proxy := NewDevProxy(target, nil)

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/items/42?q=1&r=2", strings.NewReader("payload"))
	req.Header.Set("X-Custom", "abc")
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusTeapot {
		t.Errorf("status = %d, want 418", resp.StatusCode)
	}
	if got := resp.Header.Get("X-Upstream"); got != "yes" {
		t.Errorf("X-Upstream = %q", got)
	}
	if got := body(t, resp); got != "from vite" {
		t.Errorf("body = %q", got)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q", gotMethod)
	}
	if gotPath != "/items/42" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "q=1&r=2" {
		t.Errorf("query = %q", gotQuery)
	}
	if gotBody != "payload" {
		t.Errorf("upstream body = %q", gotBody)
	}
	// Vite validates the inbound Host; it must stay the browser's, not the
	// upstream's, or `server.allowedHosts` starts mattering.
	if gotHost != "127.0.0.1:8080" {
		t.Errorf("upstream Host = %q, want the inbound host", gotHost)
	}
	if got := gotHeader.Get("X-Custom"); got != "abc" {
		t.Errorf("X-Custom = %q", got)
	}
	if got := gotHeader.Get("X-Forwarded-For"); got == "" {
		t.Error("missing X-Forwarded-For")
	}
	if got := gotHeader.Get("X-Forwarded-Host"); got != "127.0.0.1:8080" {
		t.Errorf("X-Forwarded-Host = %q", got)
	}
	if got := gotHeader.Get("X-Forwarded-Proto"); got != "http" {
		t.Errorf("X-Forwarded-Proto = %q", got)
	}
}

// TestDevProxyTunnelsUpgrade is the HMR case: Vite's client opens its
// WebSocket on the page's own port, so the upgrade arrives at Go and has to
// reach the Vite dev server intact.
func TestDevProxyTunnelsUpgrade(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			t.Errorf("upstream saw Upgrade = %q", r.Header.Get("Upgrade"))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		defer conn.Close()

		buf.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		buf.Flush()

		line, err := buf.ReadString('\n')
		if err != nil {
			t.Errorf("upstream read: %v", err)
			return
		}
		buf.WriteString("echo:" + line)
		buf.Flush()
	}))
	defer upstream.Close()

	target, _ := url.Parse(upstream.URL)

	var upgrades []string
	proxy := NewDevProxy(target, func(format string, args ...any) {
		upgrades = append(upgrades, format)
	})

	front := httptest.NewServer(proxy)
	defer front.Close()

	conn, err := net.Dial("tcp", strings.TrimPrefix(front.URL, "http://"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	io.WriteString(conn, "GET / HTTP/1.1\r\nHost: "+strings.TrimPrefix(front.URL, "http://")+
		"\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")

	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("status line = %q, want 101", status)
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("read headers: %v", err)
		}
		if line == "\r\n" {
			break
		}
	}

	io.WriteString(conn, "ping\n")
	echoed, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if echoed != "echo:ping\n" {
		t.Fatalf("echo = %q", echoed)
	}

	if len(upgrades) == 0 {
		t.Error("proxy did not log the upgrade; the dev check depends on this line")
	}
}

func TestDevProxyUnreachableTargetIs502(t *testing.T) {
	// Port 1 on loopback: nothing listens there.
	target, _ := url.Parse("http://127.0.0.1:1")
	proxy := NewDevProxy(target, nil)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	if got := body(t, resp); !strings.Contains(got, "skgo") {
		t.Errorf("body = %q, want it to name skgo", got)
	}
}
