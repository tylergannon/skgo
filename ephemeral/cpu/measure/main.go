// Local measurement of the exact handler composed by example/cmd.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"runtime/pprof"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/tylergannon/skgo/example"
	"github.com/tylergannon/skgo/example/web"
)

func cpu() time.Duration {
	var r syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &r); err != nil {
		panic(err)
	}
	return time.Duration(r.Utime.Sec+r.Stime.Sec)*time.Second + time.Duration(r.Utime.Usec+r.Stime.Usec)*time.Microsecond
}
func emit(v any) {
	if err := json.NewEncoder(os.Stdout).Encode(v); err != nil {
		panic(err)
	}
}
func main() {
	n := flag.Int("n", 200, "requests per path")
	path := flag.String("path", "", "one path; empty measures all")
	profile := flag.String("cpu", "", "warm CPU profile")
	alloc := flag.String("alloc", "", "allocation profile")
	flag.Parse()
	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		panic(err)
	}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started, used := time.Now(), cpu()
	h, mode, err := example.NewHandler(dist, "", "http://127.0.0.1:8080")
	if err != nil {
		panic(err)
	}
	runtime.ReadMemStats(&after)
	emit(map[string]any{"phase": "startup", "mode": mode, "wall_ms": float64(time.Since(started)) / 1e6, "cpu_ms": float64(cpu()-used) / 1e6, "alloc_bytes": after.TotalAlloc - before.TotalAlloc, "gomaxprocs": runtime.GOMAXPROCS(0), "cpus": runtime.NumCPU()})
	used = cpu()
	time.Sleep(2 * time.Second)
	emit(map[string]any{"phase": "idle", "seconds": 2, "cpu_ms": float64(cpu()-used) / 1e6})
	cases := []struct{ path, want string }{
		{"/", `<h1 data-testid="title">Home</h1>`},
		{"/items/93", `<p data-testid="item-name">Widget 93</p>`},
		{"/api/request-fetch", `{"fact":"harbour-lamp-4096","visitor":"guest"}`},
		{"/robots.txt", "User-agent: *"},
		{"/request-fetch/__data.json?x-sveltekit-invalidated=01", "harbour-lamp-4096"},
		{"/request-fetch", `<p data-testid="request-fetch-fact">harbour-lamp-4096</p>`},
		{"/nested-universal", `<p data-testid="nested-fact">harbour-lamp-4096</p>`},
		{"/destinations", `data-path="/api/request-fetch"`},
	}
	request := func(path, want string) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080"+path, nil)
		h.ServeHTTP(rec, req)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), want) {
			panic(fmt.Sprintf("%s status=%d missing=%s body=%.400s", path, rec.Code, want, rec.Body.String()))
		}
	}
	for _, tc := range cases {
		if *path != "" && *path != tc.path {
			continue
		}
		started, used = time.Now(), cpu()
		request(tc.path, tc.want)
		emit(map[string]any{"phase": "first", "path": tc.path, "wall_ms": float64(time.Since(started)) / 1e6, "cpu_ms": float64(cpu()-used) / 1e6})
		runtime.GC()
		var f *os.File
		if *profile != "" {
			f, err = os.Create(*profile)
			if err != nil {
				panic(err)
			}
			if err = pprof.StartCPUProfile(f); err != nil {
				panic(err)
			}
		}
		runtime.ReadMemStats(&before)
		timings := make([]int64, *n)
		started, used = time.Now(), cpu()
		for i := range *n {
			at := time.Now()
			request(tc.path, tc.want)
			timings[i] = time.Since(at).Nanoseconds()
		}
		elapsed, spent := time.Since(started), cpu()-used
		runtime.ReadMemStats(&after)
		if f != nil {
			pprof.StopCPUProfile()
			f.Close()
		}
		slices.Sort(timings)
		emit(map[string]any{"phase": "warm", "path": tc.path, "requests": *n, "wall_ms_request": float64(elapsed) / float64(*n) / 1e6, "cpu_ms_request": float64(spent) / float64(*n) / 1e6, "p95_ms": float64(timings[(*n-1)*95/100]) / 1e6, "bytes_request": (after.TotalAlloc - before.TotalAlloc) / uint64(*n), "allocs_request": (after.Mallocs - before.Mallocs) / uint64(*n)})
		for range 3 {
			runtime.GC()
			time.Sleep(25 * time.Millisecond)
		}
		runtime.ReadMemStats(&after)
		emit(map[string]any{"phase": "retained", "path": tc.path, "heap_bytes": after.HeapAlloc, "heap_objects": after.HeapObjects, "gc_cycles": after.NumGC})
		runtime.KeepAlive(h)
	}
	if *alloc != "" {
		f, err := os.Create(*alloc)
		if err != nil {
			panic(err)
		}
		if err = pprof.Lookup("allocs").WriteTo(f, 0); err != nil {
			panic(err)
		}
		f.Close()
	}
}
