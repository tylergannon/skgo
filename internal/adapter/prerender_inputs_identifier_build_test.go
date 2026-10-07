package adapter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertAuthoredInputsIdentifier(t *testing.T, fixture, language string) {
	t.Helper()
	app := filepath.Join(fixture, "web")
	receipt := filepath.Join(fixture, "identifier-receipt")
	extension := ".remote." + language
	stubPath := filepath.Join(app, "src", "lib", "fixture"+extension)
	stub, err := os.ReadFile(stubPath)
	if err != nil {
		t.Fatalf("generated %s remote stub: %v", language, err)
	}
	for _, want := range []string{
		"remoteInputs as $skgoRemoteInputs",
		"export const skgoRemoteInputs = prerender(",
		"$skgoRemoteInputs",
	} {
		if !strings.Contains(string(stub), want) {
			t.Fatalf("generated %s remote stub omitted %q:\n%s", language, want, stub)
		}
	}
	manifestBytes, err := os.ReadFile(filepath.Join(app, "skgo.remotes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Remotes []string `json:"remotes"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	const module = "src/lib/fixture.remote.go"
	var address string
	for _, remote := range manifest.Remotes {
		if strings.HasSuffix(remote, "/skgoRemoteInputs") {
			if address != "" {
				t.Fatalf("manifest repeats authored remote %s: %s", module, manifestBytes)
			}
			address = strings.TrimSuffix(remote, "/skgoRemoteInputs")
		}
	}
	if address == "" {
		t.Fatalf("manifest omitted authored module/name %s/skgoRemoteInputs: %s", module, manifestBytes)
	}
	artifact := filepath.Join(app, "build", "prerendered", "_app", "remote", filepath.FromSlash(address), "skgoRemoteInputs", "WyJhdGxhcyJd")
	contents, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatalf("native literal input-key artifact: %v", err)
	}
	want := `{"type":"result","data":"[{\"_\":1},\"body:atlas\"]"}`
	// Kit's prerender wrapper records a dependency with only _. A
	// direct remote request additionally collects implicit data.p.
	// The page and declared-input queue may race to publish either.
	withImplicit := fmt.Sprintf(`{"type":"result","data":"[{\"_\":1,\"p\":2},\"body:atlas\",{\"%s/skgoRemoteInputs/WyJhdGxhcyJd\":3},{\"v\":1}]"}`, address)
	if string(contents) != want && string(contents) != withImplicit {
		t.Fatalf("native artifact for literal module/name/key = %s; want %s", contents, want)
	}
	entries, err := os.ReadDir(filepath.Dir(artifact))
	if err != nil || len(entries) != 1 || entries[0].Name() != "WyJhdGxhcyJd" {
		t.Fatalf("native artifact set for authored remote = %v, %v; want exactly the literal atlas key", entries, err)
	}
	calls, err := os.ReadFile(receipt)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, call := range strings.Fields(string(calls)) {
		counts[call]++
	}
	if counts["producer"] != 1 || counts["body"] != 1 || len(counts) != 2 {
		t.Fatalf("build-time producer/body calls = %q; want one producer and one body", calls)
	}
}
