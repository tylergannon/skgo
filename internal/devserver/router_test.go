package devserver

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPublicRouterKeepsApplicationAndToolingOwnershipAcrossReplacement(t *testing.T) {
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Add("Set-Cookie", "one=1; Path=/")
		w.Header().Add("Set-Cookie", "two=2; Path=/")
		w.WriteHeader(201)
		fmt.Fprintf(w, "APP731 %s %s %s %s", r.Method, r.Host, r.Header.Get("Cookie"), body)
	}))
	defer app.Close()
	tools := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "VITE419 "+r.URL.RequestURI()) }))
	defer tools.Close()
	r := &router{}
	if e := r.setApp(app.URL); e != nil {
		t.Fatal(e)
	}
	if e := r.setFrontend(tools.URL, []string{"/@", "/.cache/", "/__hmr"}); e != nil {
		t.Fatal(e)
	}
	public := httptest.NewServer(r)
	defer public.Close()
	request, _ := http.NewRequest("POST", public.URL+"/orders", strings.NewReader("BODY983"))
	request.Header.Set("Cookie", "draft=601")
	res, e := http.DefaultClient.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	want := "APP731 POST " + strings.TrimPrefix(public.URL, "http://") + " draft=601 BODY983"
	if res.StatusCode != 201 || string(b) != want || len(res.Cookies()) != 2 {
		t.Fatalf("status%d cookies%v body%s", res.StatusCode, res.Cookies(), b)
	}
	for _, p := range []string{"/@vite/client", "/.cache/deps/module.js?x=2"} {
		res, e = http.Get(public.URL + p)
		if e != nil {
			t.Fatal(e)
		}
		b, _ = io.ReadAll(res.Body)
		res.Body.Close()
		if string(b) != "VITE419 "+p {
			t.Fatalf("tooling stolen: %s", b)
		}
	}
	app.Close()
	r.status("compiler.go:7: missing value <repair>")
	res, e = http.Get(public.URL + "/orders")
	if e != nil {
		t.Fatal(e)
	}
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 503 || !strings.Contains(string(b), "compiler.go:7: missing value &lt;repair&gt;") {
		t.Fatalf("unavailable status%d %s", res.StatusCode, b)
	}
	replacement := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "APP887") }))
	defer replacement.Close()
	r.setApp(replacement.URL)
	res, e = http.Get(public.URL + "/orders")
	if e != nil {
		t.Fatal(e)
	}
	b, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if string(b) != "APP887" {
		t.Fatalf("target was stale: %s", b)
	}
	r.blocked.Store(true)
	res, _ = http.Get(public.URL + "/orders")
	res.Body.Close()
	if res.StatusCode != 503 {
		t.Fatal("incoherent structural publication was served")
	}
	res, _ = http.Get(public.URL + "/@vite/client")
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("structural gate blocked tooling")
	}
}

func TestPublicUpgradeSurvivesPrivateApplicationReplacement(t *testing.T) {
	tools := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer tools.Close()
	r := &router{}
	r.setFrontend(tools.URL, []string{"/__hmr"})
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
	r.setApp("http://127.0.0.1:1")
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
