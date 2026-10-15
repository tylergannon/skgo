package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	before, e := os.ReadFile("ephemeral/experiments/20261015-coordinator-integration/runtime/ALPHA/backend-address")
	if e != nil {
		panic(e)
	}
	raw, e := os.ReadFile("ephemeral/experiments/20261015-coordinator-integration/run.log")
	if e != nil {
		panic(e)
	}
	matches := regexp.MustCompile(`HOST instance=ALPHA pid=([0-9]+) discovered=([^ ]+) public=http://127.0.0.1:56612`).FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		panic("no own current ALPHA identity")
	}
	last := matches[len(matches)-1]
	if last[2] != string(before) {
		panic("own latest backend identity mismatches marker")
	}
	pid, e := strconv.Atoi(last[1])
	if e != nil {
		panic(e)
	}
	if e = syscall.Kill(pid, syscall.SIGKILL); e != nil {
		panic(e)
	}
	fmt.Printf("KILLED exact ALPHA PID%d prior private=%s\n", pid, before)
	client := http.Client{Timeout: time.Second}
	down := false
	for end := time.Now().Add(15 * time.Second); time.Now().Before(end); {
		res, e := client.Get("http://127.0.0.1:56612/orders")
		if e == nil {
			b, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if res.StatusCode == 503 {
				if !down {
					fmt.Printf("PUBLIC_DOWN status503 body=%q\n", b)
					os.WriteFile("ephemeral/experiments/20261015-coordinator-integration/crash-503.txt", b, 0644)
				}
				down = true
			}
			after, _ := os.ReadFile("ephemeral/experiments/20261015-coordinator-integration/runtime/ALPHA/backend-address")
			if string(after) != string(before) && res.StatusCode == 200 && strings.Contains(string(b), "Go SSR total: 4101") {
				if !down {
					panic("no observed503")
				}
				fmt.Printf("RECOVERED publicsame origin=%s newprivate=%s literalSSR4101\n", res.Header.Get("X-Application-Origin"), after)
				os.WriteFile("ephemeral/experiments/20261015-coordinator-integration/crash-recovered.html", b, 0644)
				return
			}
		}
		time.Sleep(time.Millisecond)
	}
	panic("no correctly valued recovery")
}
