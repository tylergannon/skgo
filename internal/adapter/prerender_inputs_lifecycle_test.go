package adapter

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// These tests exercise the native Vite build boundary. The JavaScript below is
// only the programmatic equivalent of `vp build`; the adapter and Kit plugins
// are loaded from the disposable copy of the real example.
func TestPrerenderInputsBuildAppWaitsForProducerFailureDrain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Fatal("process-group lifecycle evidence requires Unix process groups")
	}
	fixture := prepareInputsLifecycleFixture(t, "producer-failure")
	result := runInputsBuildApp(t, fixture)
	if result.err == nil {
		t.Fatalf("native Vite buildApp resolved after producer failure; output:\n%s", result.output)
	}
	assertInputsDrainComplete(t, fixture, result.output)
}

func TestPrerenderInputsBuildAppWaitsForUnrelatedCrawlerFailureDrain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Fatal("process-group lifecycle evidence requires Unix process groups")
	}
	fixture := prepareInputsLifecycleFixture(t, "crawler-failure")
	// A real Kit page throws from its native server load. The Go-backed /about
	// prerender request is held open independently, so Vite's native crawler
	// failure must pass through the adapter's buildApp drain boundary.
	page := filepath.Join(fixture, "web", "src", "routes", "z-native-failure", "+page.server.ts")
	if err := os.MkdirAll(filepath.Dir(page), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, []byte(`import { error } from "@sveltejs/kit";
import { readFile } from "node:fs/promises";
import process from "node:process";
export const prerender = true;
export async function load() {
  const deadline = Date.now() + 20000;
  while (Date.now() < deadline) {
    try {
      const receipt = (await readFile(process.env.SKGO_LIFECYCLE_RECEIPT, "utf8")).trim().split("\n");
      const pid = Number(receipt[0]);
      if (receipt.length < 3 || !Number.isInteger(pid) || pid < 2) { await new Promise((resolve) => setTimeout(resolve, 20)); continue; }
      try { process.kill(pid, 0); }
      catch { throw new Error("Go body exited before the native crawler failure"); }
      return error(500, "native crawler fixture failure");
    }
    catch (cause) { if (cause?.status || cause?.code !== "ENOENT") throw cause; await new Promise((resolve) => setTimeout(resolve, 20)); }
  }
  throw new Error("timed out waiting for the independent Go body receipt");
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(page), "+page.svelte"), []byte(`<h1>Native crawler failure fixture</h1>`), 0o644); err != nil {
		t.Fatal(err)
	}
	result := runInputsBuildApp(t, fixture)
	if result.err == nil {
		t.Fatalf("native Vite buildApp resolved after crawler failure; output:\n%s", result.output)
	}
	if !strings.Contains(result.output, "GET /z-native-failure") || !strings.Contains(result.output, "Failed to prerender `/z-native-failure`") {
		t.Fatalf("build rejection did not preserve the native crawler failure:\n%s", result.output)
	}
	if !strings.Contains(result.output, "BUILD_APP_REJECTED:Prerendering failed") {
		t.Fatalf("buildApp did not reject with Kit's final native crawler failure:\n%s", result.output)
	}
	assertInputsDrainComplete(t, fixture, result.output)
}

func TestPrerenderInputsVPOwnerSignalDrainsBlockedProducer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Fatal("process-group lifecycle evidence requires Unix process groups")
	}
	fixture := prepareInputsLifecycleFixture(t, "blocked")
	cmd := inputsVPBuildCommand(t, fixture)
	process := startInputsTrackedCommand(t, cmd, fixture)
	waitInputsReceipt(t, fixture)
	ownerPID, err := inputsViteOwnerPID(fixture)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := os.FindProcess(ownerPID)
	if err != nil {
		t.Fatalf("find Vite process owned by vp build: %v", err)
	}
	if err := owner.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM to actual Vite process in vp build: %v", err)
	}
	waitErr := waitInputsCommand(t, process, 20*time.Second)
	if waitErr == nil {
		t.Fatalf("VP build exited successfully after SIGTERM; output:\n%s", process.output.String())
	}
	assertInputsReceiptDrained(t, fixture)
}

func TestPrerenderInputsOuterVPCLISignalDrainsBlockedProducer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Fatal("process-group lifecycle evidence requires Unix process groups")
	}
	fixture := prepareInputsLifecycleFixture(t, "blocked")
	cmd := inputsVPBuildCommand(t, fixture)
	process := startInputsTrackedCommand(t, cmd, fixture)
	waitInputsReceipt(t, fixture)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM to the outer real vp build process: %v", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	waitErr := waitInputsCommand(t, process, time.Until(deadline))
	if waitErr == nil {
		t.Fatalf("outer vp build exited successfully after SIGTERM; output:\n%s", process.output.String())
	}
	waitInputsOwnerExit(t, fixture, deadline, process.output.String())
	assertInputsReceiptDrained(t, fixture)
}

func TestPrerenderInputsOuterVPCLIExitPrecedesOwnerCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Fatal("process-group lifecycle evidence requires Unix process groups")
	}
	fixture := prepareInputsLifecycleFixture(t, "blocked")
	configPath := filepath.Join(fixture, "web", "vite.config.ts")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	// Hold only the owner's final directory removal, after its groups have
	// drained. The outer CLI can exit and its inherited pipe wait can expire
	// while Vite is still inside this cleanup. No application work is delayed.
	controlledRemoval := `import __lifecycleFS from 'node:fs';
import { syncBuiltinESMExports as __syncLifecycleFS } from 'node:module';
const __removeLifecycleDir = __lifecycleFS.rmSync;
__lifecycleFS.rmSync = function(path, options) {
  if (String(path).startsWith(process.env.TMPDIR + '/') && /\/skgo-prerender-[^/]+$/.test(String(path))) {
    __lifecycleFS.writeFileSync(process.env.SKGO_LIFECYCLE_RECEIPT + '.removing', 'groups drained');
    const deadline = Date.now() + 10000;
    while (!__lifecycleFS.existsSync(process.env.SKGO_LIFECYCLE_RECEIPT + '.allow-removal')) {
      if (Date.now() >= deadline) throw new Error('test did not release owner directory removal');
      Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, 20);
    }
  }
  return __removeLifecycleDir(path, options);
};
__syncLifecycleFS();
`
	if err := os.WriteFile(configPath, append([]byte(controlledRemoval), config...), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := inputsVPBuildCommand(t, fixture)
	process := startInputsTrackedCommand(t, cmd, fixture)
	waitInputsReceipt(t, fixture)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	if err := waitInputsCommand(t, process, time.Until(deadline)); err == nil {
		t.Fatal("outer vp build resolved after SIGTERM")
	}
	if _, err := os.Stat(filepath.Join(fixture, "owner-receipt.removing")); err != nil {
		t.Fatalf("owner did not reach controlled removal: %v\n%s", err, process.output.String())
	}
	// This is the state the old assertion mistook for a leaked directory.
	data, err := os.ReadFile(filepath.Join(fixture, "owner-receipt"))
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(fields) != 3 {
		t.Fatalf("incomplete owner receipt: %q", data)
	}
	privateDir := fields[2]
	if _, err := os.Stat(privateDir); err != nil {
		t.Fatalf("outer exit did not precede controlled owner cleanup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "owner-receipt.allow-removal"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	waitInputsOwnerExit(t, fixture, deadline, process.output.String())
	assertInputsReceiptDrained(t, fixture)
}

func waitInputsOwnerExit(t *testing.T, fixture string, deadline time.Time, output string) {
	t.Helper()
	pid, err := inputsViteOwnerPID(fixture)
	if err != nil {
		t.Fatal(err)
	}
	// The CLI is not the owner. Cmd.WaitDelay bounds inherited pipe draining,
	// so its Wait may return before Vite's parent-loss observer finishes. Wait
	// for Vite itself within the original shutdown budget; then require the
	// same exact group, directory and late-I/O assertions as every other path.
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil {
			t.Fatalf("inspect Vite owner %d: %v", pid, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("Vite owner %d did not exit within the signal shutdown budget:\n%s", pid, output)
}

func TestPrerenderInputsMalformedGoSourceFailsAndCleansPrivateDirectory(t *testing.T) {
	fixture := prepareInputsLifecycleFixture(t, "success")
	broken := filepath.Join(fixture, "internal", "skgo", "prerender", "lifecycle_broken.go")
	if err := os.WriteFile(broken, []byte("package main\nfunc malformed( {\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := runInputsBuildApp(t, fixture)
	if result.err == nil {
		t.Fatalf("native Vite buildApp succeeded with malformed Go compiler input; output:\n%s", result.output)
	}
	if !strings.Contains(result.output, "syntax error") && !strings.Contains(result.output, "unexpected") {
		t.Fatalf("compiler failure was not visible in native build output:\n%s", result.output)
	}
	assertInputsScratchEmpty(t, fixture)
}

func TestPrerenderInputsCommandEarlyStdinCloseIsTerminal(t *testing.T) {
	result := runInputsProtocolFixture(t, -1)
	if result.err == nil {
		t.Fatalf("native Vite buildApp accepted a command that closed stdin without a response; output:\n%s", result.output)
	}
	if strings.Contains(result.output, "BUILD_APP_RESOLVED") {
		t.Fatalf("buildApp reported success after early stdin close:\n%s", result.output)
	}
}

func TestPrerenderInputsValidJSONThenNonzeroIsTerminal(t *testing.T) {
	result := runInputsProtocolFixture(t, 23)
	if result.err == nil {
		t.Fatalf("native Vite buildApp accepted valid JSON followed by a nonzero command exit; output:\n%s", result.output)
	}
	if strings.Contains(result.output, "BUILD_APP_RESOLVED") {
		t.Fatalf("buildApp reported success after command exit 23:\n%s", result.output)
	}
}

func TestPrerenderInputsValidGeneratedResponseWithZeroExitResolves(t *testing.T) {
	result := runInputsProtocolFixture(t, 0)
	if result.err != nil || !strings.Contains(result.output, "BUILD_APP_RESOLVED") {
		t.Fatalf("the same generated Go protocol with zero exit did not resolve buildApp: err=%v\n%s", result.err, result.output)
	}
}

func TestPrerenderInputsFailedDrainRetainsOwnershipForRetry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Fatal("controlled process-group drain evidence requires Unix process groups")
	}
	fixture := prepareInputsLifecycleFixture(t, "controlled-drain")
	project, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	program := `import { existsSync, readFileSync } from 'node:fs';
import process from 'node:process';
import { pathToFileURL } from 'node:url';
const bridge = await import(pathToFileURL(process.env.SKGO_PRERENDER_BRIDGE));
const baseTermListeners = process.listenerCount('SIGTERM');
const owner = bridge.createPrerenderOwner();
await owner.ensureCompiled();
const invocation = owner.invoke({ kind: 'remote-inputs', module: 'src/routes/site.remote.ts', name: 'getSite' }).catch((error) => error);
const deadline = Date.now() + 15000;
let receipt = [];
while (Date.now() < deadline) {
  try { receipt = readFileSync(process.env.SKGO_LIFECYCLE_RECEIPT, 'utf8').trim().split('\n'); } catch {}
  if (receipt.length >= 3 && /^\d+$/.test(receipt[0]) && /^\d+$/.test(receipt[1]) && receipt[2]) break;
  await new Promise((resolve) => setTimeout(resolve, 10));
}

if (!(receipt.length >= 3 && /^\d+$/.test(receipt[0]) && /^\d+$/.test(receipt[1]) && receipt[2])) throw new Error('Go Inputs callback did not write a complete receipt');
const [pidText, groupText] = receipt;
const group = Number(groupText);
const primary = new Error('native lifecycle primary failure');
const realKill = process.kill;
process.kill = function (pid, signal) {
  if (Math.abs(Number(pid)) === group && (signal === 'SIGTERM' || signal === 'SIGKILL')) {
    const error = new Error('controlled signal denial'); error.code = 'EPERM'; throw error;
  }
  return realKill.call(process, pid, signal);
};
let failureRejected = false;
try {
  await owner.fail(primary);
} catch (error) {
  failureRejected = error === primary && Boolean(error.cleanupError);
}
await invocation;
const failed = {
  failureRejected,
  primaryPreserved: owner.failed === primary,
  cleanupMetadata: Boolean(owner.cleanupFailure && primary.cleanupError),
  groupRetained: owner.groups.size === 1,
  directoryRetained: existsSync(owner.dir),
  listenersRetained: process.listenerCount('SIGTERM') > baseTermListeners,
  groupAlive: (() => { try { realKill(-group, 0); return true; } catch { return false; } })()
};
process.kill = realKill;
await owner.cleanup();
const retried = {
  groups: owner.groups.size,
  directoryExists: existsSync(owner.dir),
  listeners: process.listenerCount('SIGTERM'),
  baseTermListeners,
  groupGone: (() => { try { realKill(-group, 0); return false; } catch (error) { return error.code === 'ESRCH'; } })()
};
console.log('RETRY_RESULT:' + JSON.stringify({ pid: Number(pidText), group, failed, retried }));
`
	cmd := exec.Command("node", "-e", program)
	cmd.Dir = filepath.Join(fixture, "web")
	cmd.Env = inputsTestEnv(fixture, "")
	cmd.Env = replaceEnv(cmd.Env, "SKGO_PRERENDER_BRIDGE", filepath.Join(project, "internal", "adapter", "skgo-adapter", "prerender.js"))
	process := startInputsTrackedCommand(t, cmd, fixture)
	err, timedOut := process.wait(45 * time.Second)
	output := process.output.String()
	if timedOut {
		t.Fatalf("controlled drain retry harness exceeded 45s and was terminated:\n%s", output)
	}
	if err != nil {
		t.Fatalf("controlled drain retry harness: %v\n%s", err, output)
	}
	var result struct {
		Failed struct {
			FailureRejected   bool `json:"failureRejected"`
			PrimaryPreserved  bool `json:"primaryPreserved"`
			CleanupMetadata   bool `json:"cleanupMetadata"`
			GroupRetained     bool `json:"groupRetained"`
			DirectoryRetained bool `json:"directoryRetained"`
			ListenersRetained bool `json:"listenersRetained"`
			GroupAlive        bool `json:"groupAlive"`
		} `json:"failed"`
		Retried struct {
			Groups            int  `json:"groups"`
			DirectoryExists   bool `json:"directoryExists"`
			Listeners         int  `json:"listeners"`
			BaseTermListeners int  `json:"baseTermListeners"`
			GroupGone         bool `json:"groupGone"`
		} `json:"retried"`
	}
	var line string
	for _, item := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(item, "RETRY_RESULT:") {
			line = strings.TrimPrefix(item, "RETRY_RESULT:")
		}
	}
	if line == "" {
		t.Fatalf("controlled drain harness omitted its state receipt:\n%s", output)
	}
	if err := json.Unmarshal([]byte(line), &result); err != nil {
		t.Fatalf("parse controlled drain state: %v\n%s", err, output)
	}
	if !result.Failed.FailureRejected || !result.Failed.PrimaryPreserved || !result.Failed.CleanupMetadata || !result.Failed.GroupRetained || !result.Failed.DirectoryRetained || !result.Failed.ListenersRetained || !result.Failed.GroupAlive {
		t.Fatalf("failed cleanup forgot live ownership or replaced the primary failure: %+v\n%s", result.Failed, output)
	}
	if result.Retried.Groups != 0 || result.Retried.DirectoryExists || result.Retried.Listeners != result.Retried.BaseTermListeners || !result.Retried.GroupGone {
		t.Fatalf("successful retry did not retire all owned state: %+v\n%s", result.Retried, output)
	}
}

func TestPrerenderInputsSignalRetriesRecoverableDrainFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Fatal("controlled process-group drain evidence requires Unix process groups")
	}
	fixture := prepareInputsLifecycleFixture(t, "controlled-drain")
	project, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	program := `import { existsSync, readFileSync } from 'node:fs';
import process from 'node:process';
import { pathToFileURL } from 'node:url';
const bridge = await import(pathToFileURL(process.env.SKGO_PRERENDER_BRIDGE));
const owner = bridge.createPrerenderOwner();
await owner.ensureCompiled();
const invocation = owner.invoke({ kind: 'remote-inputs', module: 'src/routes/site.remote.ts', name: 'getSite' }).catch(() => {});
let receipt;
const deadline = Date.now() + 15000;
while (Date.now() < deadline) {
  try { receipt = readFileSync(process.env.SKGO_LIFECYCLE_RECEIPT, 'utf8').trim().split('\n'); } catch {}
  if (receipt?.length === 3) break;
  await new Promise((resolve) => setTimeout(resolve, 10));
}
if (receipt?.length !== 3) throw new Error('missing Go producer receipt');
const group = Number(receipt[1]);
const realKill = process.kill;
process.kill = function(pid, signal) {
  if (Number(pid) === -group && (signal === 'SIGTERM' || signal === 'SIGKILL')) {
    const error = new Error('recoverable signal denial'); error.code = 'EPERM'; throw error;
  }
  return realKill.call(process, pid, signal);
};
let cleanupFailure;
Object.defineProperty(owner, 'cleanupFailure', {
  get() { return cleanupFailure; },
  set(error) {
    cleanupFailure = error;
    if (!(error instanceof AggregateError)) return;
    console.log('RECOVERABLE_DRAIN_FAILURE:' + JSON.stringify({
      groupAlive: (() => { try { realKill(-group, 0); return true; } catch { return false; } })(),
      directoryRetained: existsSync(owner.dir),
      groupRetained: owner.groups.size === 1,
      signalFailure: owner.failed?.message === 'skgo prerender build interrupted by SIGTERM'
    }));
    process.kill = realKill;
  }
});
// Model the active native build while its worker is waiting for Go. A failed
// drain must not require another signal or incoming request to recover.
setInterval(() => {}, 1000);
process.kill(process.pid, 'SIGTERM');
await invocation;
`
	cmd := exec.Command("node", "-e", program)
	cmd.Dir = filepath.Join(fixture, "web")
	cmd.Env = inputsTestEnv(fixture, "")
	cmd.Env = replaceEnv(cmd.Env, "SKGO_PRERENDER_BRIDGE", filepath.Join(project, "internal", "adapter", "skgo-adapter", "prerender.js"))
	process := startInputsTrackedCommand(t, cmd, fixture)
	waitInputsReceipt(t, fixture)
	if err := waitInputsCommand(t, process, 20*time.Second); err == nil {
		t.Fatal("signal-interrupted owner exited successfully")
	}
	if !strings.Contains(process.output.String(), `RECOVERABLE_DRAIN_FAILURE:{"groupAlive":true,"directoryRetained":true,"groupRetained":true,"signalFailure":true}`) {
		t.Fatalf("did not exercise retained ownership after a real drain failure:\n%s", process.output.String())
	}
	assertInputsReceiptDrained(t, fixture)
}

type inputsBuildResult struct {
	output string
	err    error
}

func prepareInputsLifecycleFixture(t *testing.T, mode string) string {
	t.Helper()
	project, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(project, "example")
	fixture := filepath.Join(t.TempDir(), "example")
	copyFixtureTree(t, source, fixture)
	dependencies := filepath.Join(source, "web", "node_modules")
	if _, err := os.Stat(filepath.Join(dependencies, ".bin", "vp")); err != nil {
		t.Fatalf("pinned frontend dependencies are required for native lifecycle evidence: %v", err)
	}
	linkFixtureDependencies(t, dependencies, filepath.Join(fixture, "web", "node_modules"), filepath.Join(project, "internal", "adapter"))
	modPath := filepath.Join(fixture, "go.mod")
	mod, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatal(err)
	}
	oldReplace := "replace github.com/tylergannon/skgo => ../"
	newReplace := "replace github.com/tylergannon/skgo => " + filepath.Clean(project)
	updated := strings.Replace(string(mod), oldReplace, newReplace, 1)
	if updated == string(mod) {
		t.Fatal("fixture module does not contain the expected local replace")
	}
	if err := os.WriteFile(modPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(fixture, "web", "vite.config.ts")
	config, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	configText := strings.Replace(string(config), "export default defineConfig({", "import { writeFileSync as __skgoWriteOwner } from 'node:fs';\n__skgoWriteOwner(process.env.SKGO_VITE_OWNER_PID, String(process.pid));\nexport default defineConfig({", 1)
	configText = strings.Replace(configText, "adapter: skgo(),", "adapter: skgo(),\n      prerender: { concurrency: 4 },", 1)
	if configText == string(config) {
		t.Fatal("could not add the disposable Vite owner PID receipt")
	}
	if err := os.WriteFile(configPath, []byte(configText), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, ".lifecycle-mode"), []byte(mode), 0o600); err != nil {
		t.Fatal(err)
	}
	inputsPath := filepath.Join(fixture, "web", "src", "routes", "site.remote.go")
	inputsSource, err := os.ReadFile(inputsPath)
	if err != nil {
		t.Fatal(err)
	}
	inputsText := string(inputsSource)
	inputsText = strings.Replace(inputsText, "func getSite(ctx context.Context) (Site, error) {\n\treturn Site{Name: \"skgo\", Colocated: \"src/routes/site.remote.go\"}, nil\n}", "func getSite(ctx context.Context, name string) (Site, error) {\n\treturn Site{Name: name, Colocated: \"src/routes/site.remote.go\"}, nil\n}", 1)
	inputsText = strings.Replace(inputsText, `"context"`, `"context"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "time"`, 1)
	inputsText = strings.Replace(inputsText, "var _ = skgo.Query(getSite)", `func siteInputs() ([]string, error) {
    mode := os.Getenv("SKGO_LIFECYCLE_MODE")
    if mode == "producer-failure" || mode == "controlled-drain" {
        if err := writeInputsLifecycleReceipt(); err != nil { return nil, err }
        if mode == "producer-failure" { time.Sleep(300 * time.Millisecond); return nil, errors.New("declared Inputs producer lifecycle failure") }
        time.Sleep(400 * time.Millisecond)
    }
    return []string{"atlas", "beacon"}, nil
}
func writeInputsLifecycleReceipt() error {
    child := exec.Command("/bin/sh", "-c", "trap '' TERM; (sleep 2; printf late > \"$1\") & wait", "sh", os.Getenv("SKGO_LIFECYCLE_LATE"))
    if os.Getenv("SKGO_LIFECYCLE_MODE") != "controlled-drain" { child.Stdout, child.Stderr = os.Stdout, os.Stderr }
    if err := child.Start(); err != nil { return err }
    exe, err := os.Executable(); if err != nil { return err }
    file, err := os.Create(os.Getenv("SKGO_LIFECYCLE_RECEIPT")); if err != nil { return err }
    _, err = fmt.Fprintf(file, "%d\n%d\n%s\n", os.Getpid(), os.Getpid(), filepath.Dir(exe))
    closeErr := file.Close(); if err != nil { return err }; return closeErr
}
var _ = skgo.Prerender(getSite, skgo.PrerenderOptions{Inputs: siteInputs})`, 1)
	if inputsText == string(inputsSource) || !strings.Contains(inputsText, "PrerenderOptions{Inputs: siteInputs}") {
		t.Fatal("could not install the declared Inputs lifecycle producer in the fixture")
	}
	if err := os.WriteFile(inputsPath, []byte(inputsText), 0o644); err != nil {
		t.Fatal(err)
	}
	if mode == "crawler-failure" || mode == "blocked" {
		loadPath := filepath.Join(fixture, "web", "src", "routes", "about", "page.server.go")
		loadSource, err := os.ReadFile(loadPath)
		if err != nil {
			t.Fatal(err)
		}
		loadText := string(loadSource)
		loadText = strings.Replace(loadText, "import (", "import (\n\t\"fmt\"\n\t\"os\"\n\t\"os/exec\"\n\t\"path/filepath\"\n\t\"time\"", 1)
		injection := "if os.Getenv(\"SKGO_LIFECYCLE_MODE\") == \"crawler-failure\" || os.Getenv(\"SKGO_LIFECYCLE_MODE\") == \"blocked\" {\n" +
			"child := exec.Command(\"/bin/sh\", \"-c\", \"trap '' TERM; (sleep 2; printf late > \\\"$1\\\") & wait\", \"sh\", os.Getenv(\"SKGO_LIFECYCLE_LATE\")); child.Stdout, child.Stderr = os.Stdout, os.Stderr; if err := child.Start(); err != nil { return PageData{}, err };\n" +
			"exe, err := os.Executable(); if err != nil { return PageData{}, err }; file, err := os.Create(os.Getenv(\"SKGO_LIFECYCLE_RECEIPT\")); if err != nil { return PageData{}, err }; _, err = fmt.Fprintf(file, \"%d\\n%d\\n%s\\n\", os.Getpid(), os.Getpid(), filepath.Dir(exe)); closeErr := file.Close(); if err != nil { return PageData{}, err }; if closeErr != nil { return PageData{}, closeErr }; <-time.After(30 * time.Second); }\n"
		const signature = "func pageLoad(event RequestEvent) (PageData, error) {"
		if !strings.Contains(loadText, signature) {
			t.Fatal("about load signature changed; lifecycle injection was not installed")
		}
		loadText = strings.Replace(loadText, signature, signature+"\n"+injection, 1)
		if err := os.WriteFile(loadPath, []byte(loadText), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	generate := exec.Command("go", "generate", "./...")
	generate.Dir = fixture
	generate.Env = inputsTestEnv(fixture, mode)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("go generate lifecycle fixture: %v\n%s", err, output)
	}
	return fixture
}

func runInputsBuildApp(t *testing.T, fixture string) inputsBuildResult {
	t.Helper()
	cmd := exec.Command("node", "-e", inputsBuildProgram())
	cmd.Dir = filepath.Join(fixture, "web")
	cmd.Env = inputsTestEnv(fixture, "")
	process := startInputsTrackedCommand(t, cmd, fixture)
	err, timedOut := process.wait(60 * time.Second)
	if timedOut {
		t.Fatalf("native buildApp harness exceeded 60s deadline and was terminated:\n%s", process.output.String())
	}
	return inputsBuildResult{output: process.output.String(), err: err}
}

func inputsBuildProgram() string {
	return "import { createBuilder } from 'vite';\n" +
		"import { existsSync, readFileSync } from 'node:fs';\n" +
		"import process from 'node:process';\n" +
		"function observation() { try { const [pid, group, privateDir] = readFileSync(process.env.SKGO_LIFECYCLE_RECEIPT, 'utf8').trim().split('\\n'); let groupAlive=true; try { process.kill(-Number(group),0); } catch (e) { if (e.code==='ESRCH') groupAlive=false; } return {pid:Number(pid),group:Number(group),groupAlive,privateDirExists:existsSync(privateDir)}; } catch { return null; } }\n" +
		"const builder = await createBuilder({ root: process.cwd(), configFile: 'vite.config.ts', logLevel: 'silent', builder: {} });\n" +
		"try { await builder.buildApp(); console.log('BUILD_APP_RESOLVED'); }\n" +
		"catch (error) { console.log('DRAIN_OBSERVATION:' + JSON.stringify(observation())); const message = String(error?.message ?? error); const stack = error?.stack; console.error('BUILD_APP_REJECTED:' + message + (stack ? '\\n' + stack : '')); process.exitCode = 1; }\n" +
		"finally { await builder.close?.(); }\n"
}

func inputsVPBuildCommand(t *testing.T, fixture string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(filepath.Join(fixture, "web", "node_modules", ".bin", "vp"), "build")
	cmd.Dir = filepath.Join(fixture, "web")
	cmd.Env = inputsTestEnv(fixture, "")
	return cmd
}

func runInputsProtocolFixture(t *testing.T, exitCode int) inputsBuildResult {
	t.Helper()
	fixture := prepareInputsLifecycleFixture(t, "success")
	packageDir := filepath.Join(fixture, "internal", "inputsprotocol")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	code := `package main
import (
    "log"
    "os"
    "github.com/tylergannon/skgo"
    generated "github.com/tylergannon/skgo/example/internal/skgo"
)
func main() {
    if err := skgo.RunPrerenderBuild(os.Stdin, os.Stdout, generated.Transport(), generated.Loads(), generated.Remotes()); err != nil { log.Fatal(err) }
    os.Exit(EXIT_CODE)
}
`
	if exitCode < 0 {
		code = "package main\nimport \"os\"\nfunc main() { os.Exit(0) }\n"
	}
	code = strings.Replace(code, "EXIT_CODE", strconv.Itoa(exitCode), 1)
	if err := os.WriteFile(filepath.Join(packageDir, "main.go"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(fixture, "web", "skgo.remotes.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["prerender"] = map[string]string{"root": "..", "package": "./internal/inputsprotocol"}
	updated, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}
	return runInputsBuildApp(t, fixture)
}

func inputsTestEnv(fixture, mode string) []string {
	env := os.Environ()
	if mode == "" {
		if data, err := os.ReadFile(filepath.Join(fixture, ".lifecycle-mode")); err == nil {
			mode = string(data)
		}
	}
	set := map[string]string{
		"GOWORK":                 "off",
		"ORIGIN":                 "http://127.0.0.1:8080",
		"TMPDIR":                 filepath.Join(fixture, "owner-tmp"),
		"SKGO_LIFECYCLE_MODE":    mode,
		"SKGO_LIFECYCLE_RECEIPT": filepath.Join(fixture, "owner-receipt"),
		"SKGO_LIFECYCLE_LATE":    filepath.Join(fixture, "late-descendant-write"),
		"SKGO_VITE_OWNER_PID":    filepath.Join(fixture, "vite-owner-pid"),
	}
	_ = os.MkdirAll(set["TMPDIR"], 0o755)
	for key, value := range set {
		env = replaceEnv(env, key, value)
	}
	return env
}

func replaceEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func waitInputsReceipt(t *testing.T, fixture string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(filepath.Join(fixture, "owner-receipt")); err == nil {
			fields := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(fields) == 3 && fields[0] != "" && fields[1] != "" && fields[2] != "" {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("Go prerender body did not write its process/private-directory receipt before deadline")
}

func inputsViteOwnerPID(fixture string) (int, error) {
	data, err := os.ReadFile(filepath.Join(fixture, "vite-owner-pid"))
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid < 2 {
		return 0, fmt.Errorf("invalid Vite owner PID receipt %q: %v", data, err)
	}
	return pid, nil
}

type inputsTrackedCommand struct {
	t       *testing.T
	cmd     *exec.Cmd
	fixture string
	output  inputsLockedBuffer
	done    chan struct{}
	waitErr error
}

type inputsLockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (buffer *inputsLockedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buf.Write(data)
}

func (buffer *inputsLockedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buf.String()
}

func startInputsTrackedCommand(t *testing.T, cmd *exec.Cmd, fixture string) *inputsTrackedCommand {
	t.Helper()
	tracked := &inputsTrackedCommand{t: t, cmd: cmd, fixture: fixture}
	cmd.Stdout = &tracked.output
	cmd.Stderr = &tracked.output
	cmd.WaitDelay = 2 * time.Second
	t.Cleanup(tracked.cleanup)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start tracked command: %v", err)
	}
	tracked.done = make(chan struct{})
	go func() {
		tracked.waitErr = cmd.Wait()
		close(tracked.done)
	}()
	return tracked
}

func (tracked *inputsTrackedCommand) stopOwnedProcesses() {
	killRecordedInputsGroup(tracked.fixture)
	if pid, err := inputsViteOwnerPID(tracked.fixture); err == nil {
		if process, err := os.FindProcess(pid); err == nil {
			_ = process.Kill()
		}
	}
	if tracked.cmd.Process != nil {
		_ = tracked.cmd.Process.Kill()
	}
}

func (tracked *inputsTrackedCommand) cleanup() {
	if tracked.done == nil {
		return
	}
	tracked.stopOwnedProcesses()
	select {
	case <-tracked.done:
	case <-time.After(10 * time.Second):
		tracked.t.Errorf("tracked command did not finish within 10s after owned processes were terminated")
	}
}

func (tracked *inputsTrackedCommand) wait(timeout time.Duration) (error, bool) {
	select {
	case <-tracked.done:
		return tracked.waitErr, false
	case <-time.After(timeout):
		tracked.stopOwnedProcesses()
		select {
		case <-tracked.done:
			return fmt.Errorf("command exceeded %s deadline", timeout), true
		case <-time.After(10 * time.Second):
			return fmt.Errorf("command did not stop within 10s after %s deadline", timeout), true
		}
	}
}

func waitInputsCommand(t *testing.T, tracked *inputsTrackedCommand, timeout time.Duration) error {
	t.Helper()
	err, timedOut := tracked.wait(timeout)
	if timedOut {
		t.Fatalf("native build did not return within %s after SIGTERM:\n%s", timeout, tracked.output.String())
	}
	return err
}

func assertInputsDrainComplete(t *testing.T, fixture, output string) {
	t.Helper()
	if !strings.Contains(output, "BUILD_APP_REJECTED:") && !strings.Contains(output, "error during build") {
		t.Fatalf("native Vite build did not expose a rejected buildApp promise:\n%s", output)
	}
	var observed map[string]any
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "DRAIN_OBSERVATION:") {
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "DRAIN_OBSERVATION:")), &observed); err != nil {
				t.Fatalf("parse native buildApp drain observation: %v", err)
			}
		}
	}
	if observed == nil || observed["groupAlive"] != false || observed["privateDirExists"] != false {
		t.Fatalf("buildApp rejected before owned group/private directory drain: observation=%v\n%s", observed, output)
	}
	assertInputsReceiptDrained(t, fixture)
}

func assertInputsReceiptDrained(t *testing.T, fixture string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixture, "owner-receipt"))
	if err != nil {
		t.Fatalf("Go body did not leave a process cleanup receipt: %v", err)
	}
	fields := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(fields) != 3 {
		t.Fatalf("Go body receipt = %q; want pid, process group, and private GOTMPDIR", data)
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil || pid < 2 {
		t.Fatalf("invalid owned Go PID in receipt %q: %v", fields[0], err)
	}
	group, err := strconv.Atoi(fields[1])
	if err != nil || group != pid {
		t.Fatalf("Go process group receipt = %q; want detached Go leader PID %d", fields[1], pid)
	}
	assertInputsProcessGroupGone(t, group)
	privateDir := fields[2]
	if _, err := os.Stat(privateDir); !os.IsNotExist(err) {
		t.Fatalf("private Go compile directory %s remains after buildApp settled: %v", privateDir, err)
	}
	if _, err := os.Stat(filepath.Join(fixture, "late-descendant-write")); !os.IsNotExist(err) {
		t.Fatalf("owned descendant performed late I/O after buildApp settled: %v", err)
	}
	assertInputsScratchEmpty(t, fixture)
}

func assertInputsScratchEmpty(t *testing.T, fixture string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(fixture, "owner-tmp"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "skgo-prerender-") {
			t.Fatalf("private prerender temporary directory %s remains after build completion", entry.Name())
		}
	}
}

func assertInputsProcessGroupGone(t *testing.T, group int) {
	t.Helper()
	err := syscall.Kill(-group, 0)
	if err == nil || !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("owned prerender process group %d is still present: %v", group, err)
	}
}

func killRecordedInputsGroup(fixture string) {
	data, err := os.ReadFile(filepath.Join(fixture, "owner-receipt"))
	if err != nil {
		return
	}
	fields := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(fields) < 2 {
		return
	}
	group, err := strconv.Atoi(fields[1])
	if err == nil && group > 1 {
		_ = syscall.Kill(-group, syscall.SIGKILL)
	}
}
