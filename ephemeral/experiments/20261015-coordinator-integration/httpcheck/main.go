package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	client := http.Client{Timeout: 45 * time.Second}
	if os.Getenv("COORD_INIT_REBUILDER") == "1" {
		res, e := client.Get("http://127.0.0.1:56612/__tool/build")
		if e != nil {
			panic(e)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 {
			panic(string(b))
		}
		fmt.Println("RESTORED_VITE_ARTIFACT_BUILDER startup handshake200")
	}
	for _, v := range []struct{ origin, label, port string }{{"http://127.0.0.1:56612", "ALPHA", "56612"}, {"http://127.0.0.1:56621", "BETA", "56621"}} {
		for _, path := range []string{"/orders", "/__tool/identity", "/@vite/client", "/__coordinator/identity"} {
			res, e := client.Get(v.origin + path)
			if e != nil {
				panic(e)
			}
			b, _ := io.ReadAll(res.Body)
			res.Body.Close()
			os.WriteFile("ephemeral/experiments/20261015-coordinator-integration/"+os.Getenv("COORD_STAGE")+v.label+strings.NewReplacer("/", "-", "@", "").Replace(path)+".txt", b, 0644)
			if res.StatusCode != 200 {
				panic(fmt.Sprintf("%s status%d %s", path, res.StatusCode, b))
			}
			switch path {
			case "/orders":
				if res.Header.Get("X-Application-Origin") != v.origin || !strings.Contains(string(b), v.label) || !strings.Contains(string(b), "Go SSR total: "+func() string {
					v := os.Getenv("COORD_EXPECT_" + v.label)
					if v == "" {
						return "3000"
					}
					return v
				}()) {
					panic("wrong app identity/value")
				}
				if os.Getenv("COORD_FINAL") == "1" {
					want := "FRONTEND_ONE"
					if v.label == "ALPHA" {
						want = "FRONTEND_TWO"
					}
					if !strings.Contains(string(b), want) {
						panic("wrong frontend SSR")
					}
				}
			case "/__tool/identity":
				if !strings.Contains(string(b), `"publicOrigin":"`+v.origin+`"`) {
					panic("wrong Kit/Vite public origin")
				}
			case "/@vite/client":
				if !strings.Contains(string(b), "const hmrPort = "+v.port) || !strings.Contains(string(b), "/__hmr") {
					panic("public socket port/path not injected")
				}
			}
			fmt.Printf("PASS %s %s publicOrigin=%s\n", v.label, path, v.origin)
		}
	}
}
