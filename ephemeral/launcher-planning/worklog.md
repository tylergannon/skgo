initial decision: Plan only issue #240 from public v0.17.0 in an isolated worktree. Parent subsequently authorized implementation, independent qualification, PR, merge and release after passing checks. The separate restricted development investigation remains excluded.
correction: This repository publishes Git tags, Go modules and npm packages without requiring a GitHub Release entry. Confirm those artifacts directly, not `/releases/latest`.
friction: A generated app nested under this repository inherits its go.work and fails `go generate ./...` because the new module is not listed -> keep native scaffold fixtures outside the repo and use GOWORK=off.
decision: Both optional launcher seams belong to the existing native @skgo/sv frontend integration, not the runtime adapter or skgo's own Go MCP server.
friction: Replacing npm in the stock Playwright command alone leaves its readiness port 4173 inconsistent with the generated Go origin; switching to webServer.url also removes Playwright's implicit baseURL injection -> repair command, readiness URL, and use.baseURL together.
decision: Use the generated project's local VP 1.0.0; this laptop's global VP is 0.3.3 and must not qualify a scaffold.
decision: Public v0.17.0 TS and JS scaffolds both reproduce zero-test stock Playwright startup failures. A separate candidate config passes two tests with a literal Go response and owned shutdown. Candidate proof is not proof of repaired stock generation.

2026-10-05 — Native launcher implementation qualification

Two clean candidate rounds failed initial frontend checks: an unbound process reference, then importing node:process without scoped Node types. Real TS and checkJs scaffold creation caught both before runtime; a Node-type-free environment lookup passed clean TS/JS creation, checking, and builds. Do not replace this proof with parser-only assertions.

Independent review found a fingerprint transcription error and authored-spread migration corruption. Exact upstream digests and ambiguous-shape preservation controls corrected them; review round 02 checks the entire current work.

Operating pitfalls: put pnpm stores outside the Go worktree (a linked package's nested Go module breaks go vet ./...); VP builtin check enforces formatting, while the generated check package script needs vp run check. Existing public JS fixture configured remote MCP; no local launcher should be invented during its migration. Candidate MCP proof comes from fresh local-client fixtures and a separate real reversed-order existing JS migration. Native SV adds preserve Go/Just/application files and second application produces identical bytes.

Negative runs that share a consumer must be sequential because just serve rebuilds its frontend and binary. The first parallel no-baseURL attempt failed startup from that collision and was discarded; the isolated retry reaches page.goto/request invalid-URL failures as required. The wrong-origin timeout, intentional browser failure cleanup, Node preview failing the literal Go receipt, and occupied fixture-owned port checks all record explicit failures; the occupied listener remains alive.

Review correction: native Playwright defineConfig is variadic, and computed properties can override literal launcher fields. Recognize only a single imported defineConfig object (or native direct/resolved object) and reject ambiguous computed/spread/duplicate launcher shapes before writing. Avoid inferring runtime meaning from the first property or argument.
