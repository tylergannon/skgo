package windowsapp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tylergannon/skgo/plugins/windows-shell/windowsapp"
	"github.com/tylergannon/skgo/templateapi"
)

func TestGeneratedHostServesApplicationAndStopsOnParentEOF(t *testing.T) {
	root := t.TempDir()
	_, err := windowsapp.Apply(t.Context(), templateapi.Request{Addon: "windows-app", Project: templateapi.Project{Root: root, Module: "example.test/application"}, Options: map[string]string{"identity": "Example.Lifetime"}})
	if err != nil {
		t.Fatal(err)
	}
	// The fixture owns only the application-handler seam; the executable and
	// ready/lifetime protocol are the exact generated host source.
	for name, text := range map[string]string{
		"go.mod": "module example.test/application\n\ngo 1.27.3\n",
		"app.go": `package app
import("io/fs"; "net/http")
func NewHandler(fs.FS,string)(http.Handler,error){return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.Write([]byte("new application handler"))}),nil}`,
		"web/dist.go": `package web
import("io/fs";"testing/fstest")
var Build fs.FS = fstest.MapFS{"build/index.html": &fstest.MapFile{Data: []byte("fixture")}}`,
	} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	binary := filepath.Join(root, "application-host.exe")
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, "./windows/host")
	build.Dir = root
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build host: %v\n%s", err, out)
	}
	cmd := exec.CommandContext(ctx, binary)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	var ready struct {
		Origin string `json:"origin"`
	}
	if err := json.NewDecoder(stdout).Decode(&ready); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(ready.Origin)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || string(body) != "new application handler" {
		t.Fatalf("handler: %d %q %v", response.StatusCode, body, err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("parent EOF: %v", err)
	}
	if response, err := client.Get(ready.Origin); err == nil {
		response.Body.Close()
		t.Fatal("application host still answers after parent EOF")
	}
}
