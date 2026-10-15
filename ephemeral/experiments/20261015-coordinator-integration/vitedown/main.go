package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func main() {
	for _, c := range []struct {
		path, want string
		status     int
	}{{"/orders", "Go SSR total: 4101", 200}, {"/@vite/client", "COORDINATOR_VITE_UNAVAILABLE", 503}} {
		res, e := http.Get("http://127.0.0.1:56612" + c.path)
		if e != nil {
			panic(e)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != c.status || !strings.Contains(string(b), c.want) {
			panic(fmt.Sprintf("wrong %s status%d body%s", c.path, res.StatusCode, b))
		}
		fmt.Printf("PASS VITE_DOWN public56612 %s status%d literal=%s\n", c.path, res.StatusCode, c.want)
		os.WriteFile("ephemeral/experiments/20261015-coordinator-integration/vite-down"+strings.ReplaceAll(c.path, "/", "-")+".txt", b, 0644)
	}
}
