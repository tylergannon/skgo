// Run from the repository root. Measures an isolated copy of the real example.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime/pprof"
	"strings"
	"time"

	"encoding/base32"
	"github.com/tylergannon/skgo/generate"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func write(path string, data []byte) { must(os.WriteFile(path, data, 0644)) }
func read(path string) []byte        { b, e := os.ReadFile(path); must(e); return b }
func main() {
	root, e := os.Getwd()
	must(e)
	sandbox, e := os.MkdirTemp("", "skgo-worker-example-")
	must(e)
	defer os.RemoveAll(sandbox)
	original := filepath.Join(root, "example")
	must(filepath.WalkDir(original, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(original, path)
		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".svelte-kit" || rel == "e2e" || rel == "tmp" || rel == "native/build") {
			return filepath.SkipDir
		}
		dst := filepath.Join(sandbox, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0644)
	}))
	must(os.Symlink(filepath.Join(original, "web/node_modules"), filepath.Join(sandbox, "web/node_modules")))
	must(os.Setenv("GOWORK", "off"))
	run := func(args ...string) ([]byte, error) {
		cmd := exec.Command("go", args...)
		cmd.Dir = sandbox
		return cmd.CombinedOutput()
	}
	polytypeRoot := os.Getenv("SKGO_WORKER_POLYTYPE")
	if polytypeRoot == "" {
		polytypeRoot = "/Users/tyler/.codex/worktrees/polytype-warm-analysis"
	}
	for _, args := range [][]string{{"mod", "edit", "-replace=github.com/tylergannon/skgo=" + root}, {"mod", "edit", "-replace=github.com/tylergannon/polytype=" + polytypeRoot}} {
		out, err := run(args...)
		if err != nil {
			panic(fmt.Sprintf("%v: %s", err, out))
		}
	}
	cfg := generate.Config{Web: filepath.Join(sandbox, "web"), Out: filepath.Join(sandbox, "internal/skgo"), CacheDir: "off", LocalsPackage: "github.com/tylergannon/skgo/example/internal/app", HookPackage: "github.com/tylergannon/skgo/example/internal/serverhooks"}
	cfg.Logf = func(format string, args ...any) {
		message := fmt.Sprintf(format, args...)
		if strings.HasPrefix(message, "generation ") {
			fmt.Println(message)
		}
	}
	var worker generate.Worker
	defer worker.Close()
	emit := func(label string, start time.Time, extra any) {
		b, _ := json.Marshal(map[string]any{"phase": label, "milliseconds": float64(time.Since(start).Microseconds()) / 1000, "stats": worker.Stats(), "detail": extra})
		fmt.Println(string(b))
	}
	generation := func(label string) { start := time.Now(); must(worker.Run(cfg)); emit(label, start, nil) }
	binary := filepath.Join(sandbox, "app")
	build := func(label string, wantError bool) {
		start := time.Now()
		out, err := run("build", "-o", binary, "./cmd")
		if wantError {
			if err == nil || !bytes.Contains(out, []byte("undefined: workerProbeMissing")) {
				panic(fmt.Sprintf("wrong build result: %v %s", err, out))
			}
		} else if err != nil {
			panic(fmt.Sprintf("build: %v %s", err, out))
		}
		emit(label, start, string(out))
	}
	link := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("src/routes/go-dev")))
	must(os.MkdirAll(filepath.Join(sandbox, "workerproof"), 0755))
	write(filepath.Join(sandbox, "workerproof/main.go"), []byte(fmt.Sprintf(`package main
import("context";"fmt";"os";route "github.com/tylergannon/skgo/example/internal/skgo/links/%s")
func main(){ got,err:=route.Skgo_getRevision(context.Background());if err!=nil || got!=os.Args[1] {panic(fmt.Sprintf("got=%%q err=%%v want=%%q",got,err,os.Args[1]))};fmt.Println(got) }
`, link)))
	checkHandler := func(want string) {
		out, err := run("run", "./workerproof", want)
		if err != nil {
			panic(fmt.Sprintf("compiled handler: %v %s", err, out))
		}
		fmt.Printf("COMPILED_HANDLER %s", out)
	}

	generation("cold-generation")
	if profile := os.Getenv("SKGO_WORKER_API_CPU"); profile != "" {
		domain := filepath.Join(sandbox, "businesslogic/store.go")
		write(domain, bytes.Replace(read(domain), []byte("type Todo struct {"), []byte("type Todo struct {\n WorkerAdded bool `json:\"workerAdded\"`"), 1))
		if trace := os.Getenv("SKGO_GO_TRACE"); trace != "" {
			write(trace, nil)
		}
		f, err := os.Create(profile)
		must(err)
		must(pprof.StartCPUProfile(f))
		generation("profile-api")
		pprof.StopCPUProfile()
		must(f.Close())
		return
	}
	if profile := os.Getenv("SKGO_WORKER_CPU"); profile != "" {
		f, err := os.Create(profile)
		must(err)
		must(pprof.StartCPUProfile(f))
		source := filepath.Join(cfg.Web, "src/routes/go-dev/go-dev.remote.go")
		initial := read(source)
		for i := 0; i < 8; i++ {
			write(source, bytes.Replace(initial, []byte("Go wire revision one"), []byte(fmt.Sprintf("Profile revision %d", i)), 1))
			generation("profile-body")
		}
		pprof.StopCPUProfile()
		must(f.Close())
		return
	}
	build("cold-build", false)
	checkHandler("Go wire revision one")
	source := filepath.Join(cfg.Web, "src/routes/go-dev/go-dev.remote.go")
	initial := read(source)
	write(source, bytes.Replace(initial, []byte("Go wire revision one"), []byte("Worker body revision two"), 1))
	generation("body-generation")
	build("body-build", false)
	checkHandler("Worker body revision two")
	write(source, bytes.Replace(initial, []byte(`"Go wire revision one"`), []byte("workerProbeMissing"), 1))
	generation("broken-body-generation")
	build("broken-body-build", true)
	write(source, initial)
	generation("repaired-body-generation")
	build("repaired-body-build", false)
	domain := filepath.Join(sandbox, "businesslogic/store.go")
	write(domain, bytes.Replace(read(domain), []byte("type Todo struct {"), []byte("type Todo struct {\n WorkerAdded bool `json:\"workerAdded\"`"), 1))
	generation("api-generation")
	projected := filepath.Join(cfg.Web, "src/lib/skgo/businesslogic/types.ts")
	if !bytes.Contains(read(projected), []byte("workerAdded")) {
		panic("API output missing literal added field")
	}
	must(os.Remove(filepath.Join(cfg.Out, "skgo_gen.go")))
	write(projected, []byte("// Code generated by skgo. DO NOT EDIT.\ninvalid"))
	generation("missing-output-generation")
	if !bytes.Contains(read(projected), []byte("workerAdded")) {
		panic("API output was not restored")
	}
	outputs := func() map[string]string {
		m := map[string]string{}
		must(filepath.WalkDir(sandbox, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if d.IsDir() {
				if d.Name() == "node_modules" || d.Name() == ".svelte-kit" {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			data, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			if bytes.HasPrefix(data, []byte("// Code generated by")) {
				rel, _ := filepath.Rel(sandbox, path)
				m[rel] = string(data)
			}
			return nil
		}))
		return m
	}
	before := outputs()
	start := time.Now()
	must(generate.Run(cfg))
	emit("authoritative-generation", start, len(before))
	after := outputs()
	if !reflect.DeepEqual(before, after) {
		var changed []string
		for path, data := range before {
			if after[path] != data {
				changed = append(changed, path)
			}
		}
		panic("authoritative output mismatch: " + strings.Join(changed, ","))
	}
	fmt.Printf("MATCHED %d generated outputs\n", len(before))
}
