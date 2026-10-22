# Generated client endpoint configuration review

Target: the current endpoint-configuration correction in `/Users/tyler/.codex/worktrees/58f1/skgo`, against Tyler's requirement that clients use the application's configured endpoint location and accept one application base URL, without an `appDir` command-line setting. The earlier approved CLI/MCP functionality and working-example definition of done remain applicable.

Final outcome: **no findings**. One reproducible URL defect found during the review was corrected and independently rechecked before this report. No remaining material defect or genuine nitpick was identified.

No implementation source was edited or committed by the reviewer. Disposable generated packages, Go overlays and binaries stayed under the example's ignored `.local` directory. The existing user-facing server on port 8080 was not contacted, restarted or stopped.

## Evidence inspected

Read the relevant complete diff and surrounding code in the template API, host generator invocation, MCP plugin/generator, HTTP client, CLI/MCP entrypoints, scaffold template, output ownership metadata, READMEs, and associated tests. Read the new adapter configuration helper and tests. Checked the pinned Kit 3.1.0 configuration loader, option validation, and SvelteKit plugin factory: `appDir` is validated in the factory and exposed through the setup plugin's `api.options`.

The final helper uses Vite's own config-file evaluator, flattens its asynchronous plugin array, and reads the options through Kit's extractor. This handles imported values and environment-dependent configuration without running the complete build-resolution hooks. The host forwards the resulting relative endpoint path; generated calls contain that path. CLI/MCP runtime configuration consists of the base URL and credentials, rather than repeating the endpoint directory.

## Checks personally run

- Plugin module: `go test -count=1 -v ./...` passed, with no skipped tests.
- Example module: `go test -count=1 -v ./...` passed, with no skipped tests.
- Root adapter test: `go test -count=1 -v ./internal/adapter -run TestRemotePathUsesEvaluatedKitConfiguration` passed, including imported and environment-selected app directories and the default. Repeated it after the final helper switched away from complete build resolution.
- Root host-forwarding test: `go test -count=1 -v ./internal/gen -run TestGeneratorReceivesConfiguredRemotePath` passed.
- Rebuilt the actual combined CLI and standalone MCP entrypoints as `.local/reviewer-endpoint-client` and `.local/reviewer-endpoint-mcp`. Both detailed help outputs expose the base URL; neither exposes origin, base-path or app-directory flags.
- After the URL correction described below, reran the complete remoteclient package tests successfully, including origin-header assertions, trailing slash, escaped interior slash and escaped trailing slash fixtures.
- `git diff --check` passed.

## Real generated-client and handler demonstration

Created an isolated Vite/Kit fixture with an imported `appDir` equal to `configured/assets` and deployment base `/shop`. Evaluated that actual fixture through the final adapter helper and asserted its result was literally `configured/assets/remote/`.

Passed that metadata through the real MCP generator, producing all three client/codec/command Go files under `.local`. A Go overlay substituted those generated files only for an isolated runtime probe. The probe served the example's actual generated task registrations through `skgo.NewRemotes` configured with the same custom endpoint directory and base path. No proxy rewrites or synthetic JSON response server were used.

Demonstrated:

1. A generated typed-client list returned an independently expected empty task list.
2. A generated Cobra command added `Custom evaluated endpoint`, returning exactly `{"result":{"id":1,"title":"Custom evaluated endpoint"}}`.
3. A generated MCP tool queried the same real handlers and returned exactly that one task.
4. The live HTTP server observed exactly three expected request paths, in order:
   - `/shop/configured/assets/remote/1al4nqo/listTasks`
   - `/shop/configured/assets/remote/1al4nqo/addTask`
   - `/shop/configured/assets/remote/1al4nqo/listTasks`

The caller supplied only the base URL with `/shop/`; the endpoint directory came from evaluated configuration and generated code. Both read and mutation paths worked. This complements the earlier independent browser/CLI/stdio-MCP example review rather than repeating that broader demonstration.

## Load-bearing checks

Used disposable Go overlays to introduce four separate regressions, each of which compiled and caused the corresponding existing test to fail:

- Return hard-coded `_app/remote/` from configuration extraction: the evaluated-configuration test failed against its literal custom path.
- Omit `RemotePath` while forwarding the host request: the host test failed because the generator received an empty path.
- Replace the generator's supplied endpoint prefix with `_app/remote/`: the generator test failed against the custom prefix in its output.
- Replace the client's received endpoint prefix with `_app/remote/`: actual transport URL assertions failed for both the ordinary and configured deployment cases.

The generator assertion alone inspects output source, but it is backed here by the compiled generated-client/real-handler demonstration above. Its success was not accepted as standalone proof.

## Defect found and resolved

The initial escaped-base implementation dropped an encoded trailing slash. For base URL `https://app.test/shop%2F`, it sent `https://app.test/shop/configured/assets/remote/hash/write`. Trimming the decoded path removed the slash represented by `%2F`, while the separately constructed escaped path retained it; the inconsistent URL fields caused Go to discard the escaped representation.

Reproduced this with a disposable overlay adding a literal case to the existing base-URL test, and reported it to the implementer. The final code now trims the escaped form and then unescapes that exact base before joining. A permanent `shop%2F` regression fixture was added. I inspected the correction and independently reran the package tests; the escaped endpoint now remains `https://app.test/shop%2F/configured/assets/remote/hash/write`.

No material findings remain. This is a review of the narrow configuration correction, not whole-project release qualification or a claim that every broader command has completed.
