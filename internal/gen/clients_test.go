package gen

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormClientGenerationRecoversAndTracksContract(t *testing.T) {
	app := sandboxExample(t)
	source := filepath.Join(app, "web", "src", "routes", "optional", "optional.remote.go")
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	second := string(raw) + "\nfunc submitAgain(ctx context.Context, in Input) (Result, error) { return submit(ctx, in) }\nvar _ = skgo.Form(submitAgain)\n"
	if err := os.WriteFile(source, []byte(second), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(app, "internal", "skgo", "client")
	files := []string{filepath.Join(dir, "skgo_client_gen.go"), filepath.Join(dir, "skgo_client_devalue_gen.go")}
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := runGoGenerate(app); err != nil {
		t.Fatalf("generate missing clients: %v\n%s", err, out)
	}
	first := make([][]byte, len(files))
	for i, file := range files {
		var err error
		first[i], err = os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Contains(first[0], []byte("SubmitAgain")) {
		t.Fatal("second supported Form has no generated client")
	}
	if bytes.Contains(first[0], []byte("SendMessage")) {
		t.Fatal("file-upload Form unexpectedly gained a scalar client")
	}
	if out, err := runGoGenerate(app); err != nil {
		t.Fatalf("repeat generation: %v\n%s", err, out)
	}
	for i, file := range files {
		again, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(again, first[i]) {
			t.Fatalf("%s changed on repeat generation", file)
		}
		if err := os.WriteFile(file, []byte("stale generated client\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := runGoGenerate(app); err != nil {
		t.Fatalf("replace stale clients: %v\n%s", err, out)
	}
	for i, file := range files {
		restored, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(restored, first[i]) {
			t.Fatalf("%s was not restored", file)
		}
	}

	changed := strings.Replace(second, "type Result struct {", "type Result struct {\n\tExtra string `json:\"extra\"`", 1)
	if changed == second {
		t.Fatal("fixture result contract changed; update test")
	}
	if err := os.WriteFile(source, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runGoGenerate(app); err != nil {
		t.Fatalf("generate changed result contract: %v\n%s", err, out)
	}
	codec, err := os.ReadFile(files[1])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(codec, first[1]) || !bytes.Contains(codec, []byte("extra")) {
		t.Fatal("changed Form result was not projected into client decoder")
	}
	if out, err := runGoBuild(filepath.Join(app, "internal", "skgo")); err != nil {
		t.Fatalf("changed client does not compile: %v\n%s", err, out)
	}
}

func TestEmptyAppRemovesOldFormClient(t *testing.T) {
	web := t.TempDir()
	if err := os.MkdirAll(filepath.Join(web, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(web, "generated")
	clientDir := filepath.Join(out, "client")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"skgo_client_gen.go", "skgo_client_devalue_gen.go"} {
		if err := os.WriteFile(filepath.Join(clientDir, name), []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := Run(Config{Web: web, Out: out}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"skgo_client_gen.go", "skgo_client_devalue_gen.go"} {
		if _, err := os.Stat(filepath.Join(clientDir, name)); !os.IsNotExist(err) {
			t.Fatalf("old client %s survived empty generation: %v", name, err)
		}
	}
}
