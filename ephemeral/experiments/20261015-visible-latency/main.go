// A bounded experiment helper: copy one example and schedule one atomic save.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	if len(os.Args) < 3 {
		panic("prepare ROOT | save KIND VALUE")
	}
	base, merr := filepath.Abs(filepath.Dir(os.Args[0]))
	must(merr)
	_ = base
	switch os.Args[1] {
	case "prepare":
		prepare(os.Args[2])
	case "save":
		save(os.Args[2], os.Args[3])
	default:
		panic("unknown command")
	}
}
func prepare(root string) {
	root, err := filepath.Abs(root)
	must(err)
	out, err := os.Getwd()
	must(err)
	app := filepath.Join(out, "app")
	must(os.MkdirAll(app, 0755))
	example := filepath.Join(root, "example")
	must(filepath.WalkDir(example, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(example, path)
		if e != nil {
			return e
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".svelte-kit", "build", "e2e", "native":
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(app, rel), 0755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		if rel == "go.mod" {
			b = []byte(strings.ReplaceAll(string(b), "=> ../", "=> "+filepath.ToSlash(root)))
		}
		return os.WriteFile(filepath.Join(app, rel), b, 0644)
	}))
	must(os.MkdirAll(filepath.Join(app, "web/build"), 0755))
	must(os.WriteFile(filepath.Join(app, "web/build/.gitkeep"), nil, 0644))
	must(os.Symlink(filepath.Join(root, "example/web/node_modules"), filepath.Join(app, "web/node_modules")))
	source := filepath.Join(app, "web/src/routes/items/[id]/item.remote.go")
	b, err := os.ReadFile(source)
	must(err)
	text := strings.Replace(string(b), "func getItem(", "var catalogueRevision=1\n\nfunc getItem(", 1)
	text = strings.Replace(text, `fmt.Sprintf("Widget %s", id)`, `fmt.Sprintf("GO_1000 %s revision %d", id, catalogueRevision)`, 1)
	must(os.WriteFile(source, []byte(text), 0644))
	must(os.WriteFile("initial-go.txt", []byte(text), 0644))
	page := filepath.Join(app, "web/src/routes/items/[id]/+page.svelte")
	b, err = os.ReadFile(page)
	must(err)
	text = strings.Replace(string(b), "</script>", "let latencyDraft=$state('');let latencyCount=$state(0);\n</script>", 1)
	text = strings.Replace(text, "<h1 data-testid=\"title\">Item {params.id}</h1>", "<h1 data-testid=\"title\">SVELTE_1000 {params.id}</h1>\n<label>Latency draft<input aria-label=\"latency draft\" bind:value={latencyDraft}></label><button onclick={()=>latencyCount++}>Latency count {latencyCount}</button>", 1)
	must(os.WriteFile(page, []byte(text), 0644))
	must(os.WriteFile("initial-page.txt", []byte(text), 0644))
	fmt.Println(app)
}
func save(kind, value string) {
	work, err := os.Getwd()
	must(err)
	path := filepath.Join(work, "app/web/src/routes/items/[id]/item.remote.go")
	if kind == "svelte" {
		path = filepath.Join(work, "app/web/src/routes/items/[id]/+page.svelte")
	}
	b, err := os.ReadFile(path)
	must(err)
	text := string(b)
	switch kind {
	case "go":
		start := strings.Index(text, `fmt.Sprintf("GO_`)
		if start < 0 {
			panic("missing Go literal")
		}
		end := strings.Index(text[start:], ` %s revision`)
		text = text[:start] + `fmt.Sprintf("GO_` + value + text[start+len(`fmt.Sprintf("GO_`)+strings.Index(text[start+len(`fmt.Sprintf("GO_`):], ` %s revision`):]
		_ = end
	case "svelte":
		start := strings.Index(text, ">SVELTE_")
		if start < 0 {
			panic("missing Svelte literal")
		}
		end := strings.Index(text[start:], " {params.id}")
		text = text[:start] + ">SVELTE_" + value + text[start+end:]
	case "structural":
		start := strings.Index(text, "var catalogueRevision")
		if start < 0 {
			panic("missing initializer")
		}
		end := strings.Index(text[start:], "\n")
		text = text[:start] + "var catalogueRevision = " + value + text[start+end:]
	default:
		panic("unknown edit kind")
	}
	tmp := filepath.Join(work, "scheduled-save.tmp")
	must(os.WriteFile(tmp, []byte(text), 0644))
	f, err := os.OpenFile(tmp, os.O_RDWR, 0644)
	must(err)
	must(f.Sync())
	must(f.Close())
	armed := time.Now()
	at := armed.Add(10 * time.Second)
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"event": "armed", "kind": kind, "value": value, "at_ms": at.UnixMilli()})
	time.Sleep(time.Until(at))
	start := time.Now()
	must(os.Rename(tmp, path))
	end := time.Now()
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"event": "saved", "kind": kind, "value": value, "start_ms": start.UnixMilli(), "end_ms": end.UnixMilli(), "start_ns": start.UnixNano(), "end_ns": end.UnixNano(), "sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(text)))})
}
