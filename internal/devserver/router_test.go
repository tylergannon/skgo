package devserver

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func privateHTTP(t *testing.T, handler http.Handler) (string, *http.Server) {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "skgo-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "server.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler}
	go server.Serve(ln)
	t.Cleanup(func() { server.Close() })
	return socket, server
}
func TestPublicRouterUsesKitForPagesDataActionsEndpointsAndGoForRemotes(t *testing.T) {
	app, _ := privateHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Add("Set-Cookie", "one=1; Path=/")
		w.Header().Add("Set-Cookie", "two=2; Path=/")
		w.WriteHeader(201)
		fmt.Fprintf(w, "GO731 %s %s %s %s", r.Method, r.Host, r.Header.Get("Cookie"), body)
	}))
	front, _ := privateHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "KIT419 %s %s", r.URL.RequestURI(), r.Header.Get("X-Skgo-Client-Address"))
	}))
	route := &router{}
	if err := route.setApp(app); err != nil {
		t.Fatal(err)
	}
	if err := route.setFrontend(front, nil); err != nil {
		t.Fatal(err)
	}
	public := httptest.NewServer(route)
	defer public.Close()
	req, _ := http.NewRequest("POST", public.URL+"/_app/remote/query", strings.NewReader("BODY983"))
	req.Header.Set("Cookie", "draft=601")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	want := "GO731 POST " + strings.TrimPrefix(public.URL, "http://") + " draft=601 BODY983"
	if res.StatusCode != 201 || string(raw) != want || len(res.Cookies()) != 2 {
		t.Fatalf("status=%d body=%s cookies=%v", res.StatusCode, raw, res.Cookies())
	}
	route.setRemotePrefix("/base/custom/remote/")
	res, err = http.Get(public.URL + "/base/custom/remote/query")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 201 || !strings.HasPrefix(string(raw), "GO731 GET ") {
		t.Fatalf("configured base/appDir: status=%d body=%s", res.StatusCode, raw)
	}
	for _, path := range []string{"/orders", "/orders/__data.json", "/orders?/save", "/api/endpoint", "/@vite/client"} {
		req, _ := http.NewRequest("GET", public.URL+path, nil)
		req.Header.Set("X-Skgo-Client-Address", "FORGED_882")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if string(raw) != "KIT419 "+path+" 127.0.0.1" {
			t.Fatalf("Kit ownership/context: %s", raw)
		}
	}
}
func TestPrivateInstancesAndSameSocketReplacement(t *testing.T) {
	a, server := privateHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ALPHA4101") }))
	b, _ := privateHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "BETA5202") }))
	r := &router{}
	r.setFrontend(a, nil)
	public := httptest.NewServer(r)
	defer public.Close()
	get := func(want string) {
		t.Helper()
		res, err := http.Get(public.URL)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if string(raw) != want {
			t.Fatalf("got %s want %s", raw, want)
		}
	}
	get("ALPHA4101")
	r.setFrontend(b, nil)
	get("BETA5202")
	r.setFrontend(a, nil)
	get("ALPHA4101")
	server.Close()
	ln, err := net.Listen("unix", a)
	if err != nil {
		t.Fatal(err)
	}
	replacement := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "REPLACED7407") })}
	go replacement.Serve(ln)
	defer replacement.Close()
	get("REPLACED7407")
}

func TestPublicUpgradeSurvivesPrivateApplicationReplacement(t *testing.T) {
	socket, _ := privateHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Sec-WebSocket-Protocol") != "vite-hmr" || r.URL.Query().Get("token") != "449" {
			http.Error(w, "wrong transport", 400)
			return
		}
		c, b, e := w.(http.Hijacker).Hijack()
		if e != nil {
			return
		}
		defer c.Close()
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

	r := &router{}
	r.setFrontend(socket, nil)
	public := httptest.NewServer(r)
	defer public.Close()
	c, e := net.Dial("tcp", strings.TrimPrefix(public.URL, "http://"))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprintf(c, "GET /__hmr?token=449 HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Protocol: vite-hmr\r\n\r\n", strings.TrimPrefix(public.URL, "http://"))
	reader := bufio.NewReader(c)
	response, e := http.ReadResponse(reader, nil)
	if e != nil || response.StatusCode != 101 {
		t.Fatalf("upgrade %v %v", response, e)
	}
	r.setApp(filepath.Join(t.TempDir(), "unavailable.sock"))
	fmt.Fprintln(c, "AFTER_APP_REPLACEMENT983")
	got, e := reader.ReadString('\n')
	if e != nil || got != "AFTER_APP_REPLACEMENT983\n" {
		t.Fatalf("socket lost on app replacement: %q %v", got, e)
	}
}
func TestListenerOwnsActualPortWhenRequestedPortOccupied(t *testing.T) {
	first, e := listen("127.0.0.1:0", false)
	if e != nil {
		t.Fatal(e)
	}
	defer first.Close()
	second, e := listen(first.Addr().String(), false)
	if e != nil {
		t.Fatal(e)
	}
	defer second.Close()
	if first.Addr().String() == second.Addr().String() {
		t.Fatal("same reserved port")
	}
	if _, e = listen(first.Addr().String(), true); e == nil {
		t.Fatal("strict occupied port silently moved")
	}
}
