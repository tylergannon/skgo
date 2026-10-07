package adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestKitQueueCompatibilityAssetsAreCanonical(t *testing.T) {
	t.Parallel()
	metadata, patch, err := KitQueueCompatibility()
	if err != nil {
		t.Fatal(err)
	}
	want := KitQueueMetadata{
		Package:         "@sveltejs/kit",
		Version:         "3.0.0",
		QueuePath:       "src/core/postbuild/queue.js",
		StockSHA256:     "dbe8bbacab35cbd119d6f5bcf9b527ce0f7a1bfa63954b5ebe047df49f46c6d5",
		CorrectedSHA256: "400bf34544e2b3c5512e489eb5a0ca99a7483f0b6d733f70cab9e81e28f3a0f5",
		PatchSHA256:     "0d35d370d3e5fb6e0c801cd1079013b3d487d6e301e27777f26fabbf6e0106c4",
		CorrectionID:    "skgo-kit-3.0.0-prerender-queue-v1",
	}
	if metadata != want {
		t.Fatalf("Kit queue metadata = %#v, want %#v", metadata, want)
	}
	if got := sha256Hex(patch); got != metadata.PatchSHA256 {
		t.Fatalf("canonical queue patch SHA-256 = %s, want %s", got, metadata.PatchSHA256)
	}
	metadataBytes, err := files.ReadFile(filepath.ToSlash(filepath.Join(filesDir, "compat", "kit-3.0.0-queue.json")))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(metadataBytes, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["correctionID"] != metadata.CorrectionID || raw["patchSHA256"] != metadata.PatchSHA256 {
		t.Fatalf("embedded metadata does not describe returned patch: %s", metadataBytes)
	}
}

func TestPrerenderInputsRejectsUnpatchedPinnedKitBeforeHTTP(t *testing.T) {
	t.Parallel()
	metadata, patch, err := KitQueueCompatibility()
	if err != nil {
		t.Fatal(err)
	}
	stock := pinnedStockQueue(t, metadata, patch)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("Node is required to execute the prerender queue guard: %v", err)
	}
	appRoot := filepath.Join(t.TempDir(), "app")
	kitRoot := filepath.Join(appRoot, "node_modules", "@sveltejs", "kit")
	writeQueueFixture(t, filepath.Join(kitRoot, "package.json"), []byte(`{"name":"@sveltejs/kit","version":"3.0.0"}`))
	writeQueueFixture(t, filepath.Join(kitRoot, filepath.FromSlash(metadata.QueuePath)), stock)
	adapterRoot := filepath.Join(appRoot, "node_modules", "@skgo", "sveltekit-adapter")
	writeQueueFixture(t, filepath.Join(adapterRoot, "package.json"), []byte(`{"name":"@skgo/sveltekit-adapter","exports":{"./package.json":"./package.json"}}`))
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	writeQueueFixture(t, filepath.Join(adapterRoot, "skgo-adapter", "compat", "kit-3.0.0-queue.json"), metadataBytes)
	program := `
import { pathToFileURL } from 'node:url';
const { remoteInputs } = await import(pathToFileURL(process.env.SKGO_PRERENDER_MODULE).href);
try {
  await remoteInputs('fixture', 'declared');
  throw new Error('remoteInputs unexpectedly accepted the incompatible Kit queue');
} catch (error) {
  if (error.name !== 'SKGO_KIT_PRERENDER_QUEUE' || error.code !== 'SKGO_KIT_PRERENDER_QUEUE') throw error;
  if (!error.message.includes(process.env.SKGO_STOCK_SHA256) || !error.message.includes(process.env.SKGO_CORRECTED_SHA256)) throw error;
  if (!error.message.includes('@sveltejs/kit@3.0.0')) throw error;
  if (!error.message.includes('kit-patch --web web --apply') || !error.message.includes('kit-patch --web web --check')) throw error;
  console.log('incompatible Kit queue rejected before HTTP with setup guidance');
}
`
	modulePath, err := filepath.Abs("skgo-adapter/prerender.js")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "--input-type=module", "-e", program)
	cmd.Dir = appRoot
	cmd.Env = append(os.Environ(), "SKGO_PRERENDER_MODULE="+modulePath,
		"SKGO_STOCK_SHA256="+metadata.StockSHA256,
		"SKGO_CORRECTED_SHA256="+metadata.CorrectedSHA256)
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "incompatible Kit queue rejected before HTTP with setup guidance") {
		t.Fatalf("prerender queue guard: err=%v output=%s", err, output)
	}
}

func TestKitQueuePatchMatchesPinnedSourceAndChangesQueueBehavior(t *testing.T) {
	t.Parallel()
	metadata, patch, err := KitQueueCompatibility()
	if err != nil {
		t.Fatal(err)
	}
	kitRoot, err := filepath.Abs(filepath.Join("../../example/web/node_modules", metadata.Package))
	if err != nil {
		t.Fatal(err)
	}
	packageBytes, err := os.ReadFile(filepath.Join(kitRoot, "package.json"))
	if err != nil {
		t.Fatalf("read installed pinned Kit package: %v", err)
	}
	var pkg struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(packageBytes, &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Name != metadata.Package || pkg.Version != metadata.Version {
		t.Fatalf("installed Kit = %s@%s; expected %s@%s", pkg.Name, pkg.Version, metadata.Package, metadata.Version)
	}
	installedQueue, err := os.ReadFile(filepath.Join(kitRoot, filepath.FromSlash(metadata.QueuePath)))
	if err != nil {
		t.Fatalf("read installed Kit queue source: %v", err)
	}
	installedHash := sha256Hex(installedQueue)
	if installedHash != metadata.StockSHA256 && installedHash != metadata.CorrectedSHA256 {
		t.Fatalf("installed Kit queue SHA-256 %s is neither stock %s nor corrected %s", installedHash, metadata.StockSHA256, metadata.CorrectedSHA256)
	}
	stock := append([]byte(nil), installedQueue...)
	stockRoot := filepath.Join(t.TempDir(), "stock")
	if installedHash == metadata.CorrectedSHA256 {
		stockPath := filepath.Join(stockRoot, filepath.FromSlash(metadata.QueuePath))
		writeQueueFixture(t, stockPath, installedQueue)
		patchPath := filepath.Join(t.TempDir(), "kit-3.0.0-queue.patch")
		writeQueueFixture(t, patchPath, patch)
		apply := exec.Command("git", "apply", "--reverse", patchPath)
		apply.Dir = stockRoot
		if output, err := apply.CombinedOutput(); err != nil {
			t.Fatalf("recover stock pinned Kit queue from corrected installation: %v\n%s", err, output)
		}
		stock, err = os.ReadFile(stockPath)
		if err != nil {
			t.Fatal(err)
		}
		if got := sha256Hex(stock); got != metadata.StockSHA256 {
			t.Fatalf("reverse-patched stock queue SHA-256 = %s, want %s", got, metadata.StockSHA256)
		}
	}

	stockRoot = filepath.Join(t.TempDir(), "stock-control")
	stockPath := filepath.Join(stockRoot, filepath.FromSlash(metadata.QueuePath))
	writeQueueFixture(t, stockPath, stock)
	patchPath := filepath.Join(t.TempDir(), "kit-3.0.0-queue.patch")
	writeQueueFixture(t, patchPath, patch)
	applyCheck := exec.Command("git", "apply", "--check", patchPath)
	applyCheck.Dir = stockRoot
	if output, err := applyCheck.CombinedOutput(); err != nil {
		t.Fatalf("canonical queue patch does not apply to pinned stock source: %v\n%s", err, output)
	}
	apply := exec.Command("git", "apply", patchPath)
	apply.Dir = stockRoot
	if output, err := apply.CombinedOutput(); err != nil {
		t.Fatalf("apply canonical queue patch to disposable pinned source: %v\n%s", err, output)
	}
	corrected, err := os.ReadFile(stockPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := sha256Hex(corrected); got != metadata.CorrectedSHA256 {
		t.Fatalf("patched queue SHA-256 = %s, want %s", got, metadata.CorrectedSHA256)
	}

	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("Node is required to execute the pinned Kit queue behavior controls: %v", err)
	}
	for _, fixture := range []struct {
		name string
		path string
		mode string
		want string
	}{
		{name: "stock negative", path: stockPath, mode: "stock", want: "stock idle-gap rejection confirmed"},
		{name: "corrected positive", path: stockPath, mode: "corrected", want: "corrected queue controls passed"},
	} {
		if fixture.mode == "corrected" {
			fixture.path = stockPath
			// The patch has already transformed stockPath. Use the corrected bytes
			// from its actual resulting file; the stock control receives its own copy.
		}
		if fixture.mode == "stock" {
			fixture.path = filepath.Join(t.TempDir(), "stock", filepath.FromSlash(metadata.QueuePath))
			writeQueueFixture(t, fixture.path, stock)
		}
		packageJSON := filepath.Join(filepath.Dir(fixture.path), "package.json")
		writeQueueFixture(t, packageJSON, []byte(`{"type":"module"}`))
		cmd := exec.Command(node, "--input-type=module", "-e", kitQueueBehaviorProgram, fixture.path, fixture.mode)
		cmd.Dir = filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(fixture.path))))
		output, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(output), fixture.want) {
			t.Errorf("%s queue behavior: err=%v output=%s", fixture.name, err, output)
		}
	}
}

const kitQueueBehaviorProgram = `
import { pathToFileURL } from 'node:url';
const { queue } = await import(pathToFileURL(process.argv[1]).href);
const mode = process.argv[2];
const fail = (message) => { throw new Error(message); };

if (mode === 'stock') {
  const q = queue(1);
  if (await q.add(async () => 'first') !== 'first') fail('first task value changed');
  await new Promise((resolve) => setImmediate(resolve));
  let message = '';
  try { await q.add(async () => 'second'); } catch (error) { message = error.message; }
  if (message !== 'Cannot add tasks to a queue that has ended') fail('stock queue did not expose the idle-gap control: ' + message);
  console.log('stock idle-gap rejection confirmed');
} else {
  const q = queue(1);
  if (await q.add(async () => 'first') !== 'first') fail('first task value changed');
  await new Promise((resolve) => setImmediate(resolve));
  if (await q.add(async () => 'second') !== 'second') fail('second task value changed during idle gap');
  await q.done();

  await queue(1).done();
  const empty = queue(1);
  await empty.done();
  let emptyRejected = false;
  try { empty.add(async () => 'late'); } catch (error) { emptyRejected = error.message === 'Cannot add tasks to a queue that has ended'; }
  if (!emptyRejected) fail('empty done() did not close the queue');

  const recursive = queue(1);
  let child;
  let announceParent;
  let releaseParent;
  const parentStarted = new Promise((resolve) => { announceParent = resolve; });
  const parentRelease = new Promise((resolve) => { releaseParent = resolve; });
  const parent = recursive.add(async () => { announceParent(); await parentRelease; child = recursive.add(async () => 'child'); return 'parent'; });
  await parentStarted;
  const recursiveDone = recursive.done();
  releaseParent();
  if (await parent !== 'parent') fail('active task value changed');
  await recursiveDone;
  if (!child || await child !== 'child') fail('active task could not enqueue after done()');

  const overlap = queue(1);
  let announcePage;
  let announceProducer;
  let finishProducer;
  const pageStarted = new Promise((resolve) => { announcePage = resolve; });
  const producerStarted = new Promise((resolve) => { announceProducer = resolve; });
  const producerFinished = new Promise((resolve) => { finishProducer = resolve; });
  const events = [];
  const page = overlap.add(async () => { events.push('body:start'); announcePage(); await producerFinished; events.push('body:end'); return 'body'; });
  const producer = (async () => {
    await pageStarted;
    events.push('producer:start');
    announceProducer();
    await new Promise((resolve) => setTimeout(resolve, 10));
    const input = overlap.add(async () => 'input');
    events.push('producer:end');
    finishProducer();
    return input;
  })();
  await producerStarted;
  const input = await producer;
  if (await page !== 'body') fail('overlapped body result changed');
  await overlap.done();
  if (await input !== 'input') fail('overlapped Inputs task result changed');
  if (events.join(',') !== 'body:start,producer:start,producer:end,body:end') fail('producer/body overlap order changed: ' + events.join(','));

  const rejected = queue(1);
  const expected = new Error('early task failure');
  const task = rejected.add(async () => { throw expected; });
  let taskError;
  try { await task; } catch (error) { taskError = error; }
  let doneError;
  try { await rejected.done(); } catch (error) { doneError = error; }
  if (taskError !== expected || doneError !== expected) fail('early task rejection identity changed');
  console.log('corrected queue controls passed');
}
`

func writeQueueFixture(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
}

func pinnedStockQueue(t *testing.T, metadata KitQueueMetadata, patch []byte) []byte {
	t.Helper()
	kitRoot, err := filepath.Abs(filepath.Join("../../example/web/node_modules", metadata.Package))
	if err != nil {
		t.Fatal(err)
	}
	packageBytes, err := os.ReadFile(filepath.Join(kitRoot, "package.json"))
	if err != nil {
		t.Fatalf("read installed pinned Kit package: %v", err)
	}
	var pkg struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(packageBytes, &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Name != metadata.Package || pkg.Version != metadata.Version {
		t.Fatalf("installed Kit = %s@%s; expected %s@%s", pkg.Name, pkg.Version, metadata.Package, metadata.Version)
	}
	installed, err := os.ReadFile(filepath.Join(kitRoot, filepath.FromSlash(metadata.QueuePath)))
	if err != nil {
		t.Fatalf("read installed Kit queue source: %v", err)
	}
	switch got := sha256Hex(installed); got {
	case metadata.StockSHA256:
		return installed
	case metadata.CorrectedSHA256:
		stockRoot := t.TempDir()
		stockPath := filepath.Join(stockRoot, filepath.FromSlash(metadata.QueuePath))
		patchPath := filepath.Join(t.TempDir(), "kit-3.0.0-queue.patch")
		writeQueueFixture(t, stockPath, installed)
		writeQueueFixture(t, patchPath, patch)
		apply := exec.Command("git", "apply", "--reverse", patchPath)
		apply.Dir = stockRoot
		if output, err := apply.CombinedOutput(); err != nil {
			t.Fatalf("recover stock pinned Kit queue in disposable fixture: %v\n%s", err, output)
		}
		stock, err := os.ReadFile(stockPath)
		if err != nil {
			t.Fatal(err)
		}
		if got := sha256Hex(stock); got != metadata.StockSHA256 {
			t.Fatalf("recovered stock queue SHA-256 = %s, want %s", got, metadata.StockSHA256)
		}
		return stock
	default:
		t.Fatalf("installed Kit queue SHA-256 %s is neither stock %s nor corrected %s", got, metadata.StockSHA256, metadata.CorrectedSHA256)
		return nil
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
