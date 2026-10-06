package adapter

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrerenderCallbackTimeoutFailsBuildDespiteApplicationCatch(t *testing.T) {
	fixture := prepareMinimalInputsApp(t)
	writeFixtureFile(t, filepath.Join(fixture, "web", "src", "routes", "timeout.remote.go"), `package routes
import (
 "context"
 "fmt"
 "os"
 "path/filepath"
 "github.com/tylergannon/skgo"
)
func blocked(ctx context.Context, name string) (string, error) {
 exe, err := os.Executable(); if err != nil { return "", err }
 receipt := fmt.Sprintf("%d\n%d\n%s\n", os.Getpid(), os.Getpid(), filepath.Dir(exe))
 if err := os.WriteFile(os.Getenv("SKGO_LIFECYCLE_RECEIPT"), []byte(receipt), 0600); err != nil { return "", err }
 <-ctx.Done()
 if err := os.WriteFile(os.Getenv("SKGO_TIMEOUT_CANCELLED"), []byte("cancelled\n"), 0600); err != nil { return "", err }
 return "", ctx.Err()
}
func noInputs() ([]string, error) { return []string{}, nil }
var _ = skgo.Prerender(blocked, skgo.PrerenderOptions{Inputs: noInputs})
`)
	writeFixtureFile(t, filepath.Join(fixture, "web", "src", "routes", "+page.ts"), `import { blocked } from './timeout.remote';
export const prerender = true;
export async function load() {
 try { await blocked('atlas'); return { message: 'unexpected success' }; }
 catch (error) {
  console.error('APPLICATION_CAUGHT_CALLBACK:' + (error instanceof Error ? error.message : String(error)));
  return { message: 'usable fallback after the callback failed' };
 }
}
`)
	writeFixtureFile(t, filepath.Join(fixture, "web", "src", "routes", "+page.svelte"), `<script lang="ts">let { data } = $props();</script><h1>{data.message}</h1>`)
	cancelled := filepath.Join(fixture, "callback-cancelled")
	env := replaceEnv(inputsTestEnv(fixture, "timeout"), "SKGO_TIMEOUT_CANCELLED", cancelled)
	generate := exec.Command("go", "generate", "./...")
	generate.Dir, generate.Env = fixture, env
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate timeout fixture: %v\n%s", err, output)
	}
	cmd := exec.Command("node", "-e", inputsBuildProgram())
	cmd.Dir, cmd.Env = filepath.Join(fixture, "web"), env
	process := startInputsTrackedCommand(t, cmd, fixture)
	err, timedOut := process.wait(60 * time.Second)
	output := process.output.String()
	t.Logf("native timeout build output:\n%s", output)
	if timedOut || err == nil {
		t.Fatalf("build swallowed the callback timeout: error=%v deadline=%v\n%s", err, timedOut, output)
	}
	const timeout = "remote src/routes/timeout.remote.ts/blocked timed out after 30000ms"
	rejectedHelper := strings.Contains(output, "BUILD_APP_REJECTED:skgo prerender service exited unexpectedly: 1") ||
		strings.Contains(output, "BUILD_APP_REJECTED:skgo prerender service exited unexpectedly: SIGKILL")
	if !strings.Contains(output, "APPLICATION_CAUGHT_CALLBACK:skgo prerender "+timeout) || !rejectedHelper {
		t.Fatalf("fixture did not catch the actual timeout before the owner rejected the build:\n%s", output)
	}
	if data, err := os.ReadFile(cancelled); err != nil || string(data) != "cancelled\n" {
		t.Fatalf("timed out callback did not cancel the Go HTTP request: %q %v\n%s", data, err, output)
	}
	assertInputsDrainComplete(t, fixture, output)
}
