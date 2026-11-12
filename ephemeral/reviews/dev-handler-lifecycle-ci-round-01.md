# Independent bounded CI repair review — 2026-11-12

Outcome: **no findings**.

Reviewer: native independent agent `/root/ci_validation`, inherited **gpt-6.1-sol**, with no model override. The preceding `agent` CLI session failed authentication before any review; it is not counted as validation. This reviewer performed the work directly without delegation and edited only this artifact.

## Target and inspected evidence

Reviewed the complete current bounded repair atop `337daf20fd21526e1309623f36118dcafe23da5c` against `ephemeral/dev-handler-lifecycle-delivery-outcomes.json`, the selected task/definition of done supplied in conversation, repository instructions, the full relevant working diff and surrounding fixture/build code. Read the complete native coexistence fixture, provisional-host archive/cache/proxy helpers, capability fixture, plugin builder's dependency/source isolation, qualification workflow, native exact-version guard and pins, and the final bounded CI worklog section. Also inspected the inherited actual execution records: the initial native pass, the subsequent capability archive failure and go.sum integrity failure, and the final later passing native/capability/integrity results.

The only implementation changes are the three qualification compiler values, dependency preparation in the owning fixture, and exclusion of live `.gimbal/` metadata from its provisional module archive. The worklog is appended. The lifecycle implementation, native guard, regression selectors and native plugin assertions are unchanged. No issues were hidden through test removal, skips, assertion weakening, fabricated dependency packages or a consumer network fallback.

## Independent commands and actual observations

1. `actionlint .github/workflows/qualification.yml`: exit **0**. An inline Ruby YAML/Shellwords check independently inspected each setup-go step, compared every job run command to HEAD, and matched the actual regular expressions against the literal regression names: exit **0**. `compiler-linux`, `compiler-macos` and `development` each select exactly `1.27.3`; Linux/macOS still select `TestHostReloadRetiresIdleApplicationResource`; development still selects `TestRealSessionRouteEditDuringPendingLoad`. Source inspection confirms `internal/hotreload/toolchain.go` still rejects any version other than `go` plus `internal/toolchain.Go`, whose value remains `1.27.3`.

2. Independently compiled the current real test package before creating my dependency cache: `GOTOOLCHAIN=local go test -tags skgo_qualification -c -o /tmp/skgo-ci-review.ue3UPS/skgo.test ./cmd/skgo`, exit **0**. Actual compiler: **go1.27.4 darwin/arm64**, the current stable compiler used by the affected Windows-plugin contracts workflow. This complements the implementer's Go 1.27.3 final pass rather than claiming native-development qualification on 1.27.4.

3. Created `/tmp/skgo-ci-review.ue3UPS/outer-cache` after compilation and independently asserted it contained **zero entries**. From `cmd/skgo`, with `GOTOOLCHAIN=local` and `GOMODCACHE=/tmp/skgo-ci-review.ue3UPS/outer-cache`, ran `/tmp/skgo-ci-review.ue3UPS/skgo.test -test.run '^TestNativeShellPluginsCoexist$' -test.count=1 -test.v -test.timeout=10m`. Exit **0**, one populated RUN/PASS, **26.28s**, no skips. Observed the unchanged final fixture log confirming both real plugins load in either order, repeated generation/SDK export preserve peer outputs and authored files, and the plugins bundle the same native core with distinct language SDKs. The exact executed fixture requires a successful consumer `go mod tidy` before either real plugin build; then requires both real `pluginbuild.Build` calls, digest-checked installation, all four peer probes across both orders, typed Swift/C# output, repeated-generation equality, SDK contents and authored-file protections. There is no skip or successful early exit that can bypass those operations.

4. With the same now-populated outer cache and **GOPROXY=off**, ran `/tmp/skgo-ci-review.ue3UPS/skgo.test -test.run '^TestCapabilityPluginActualCLI$' -test.count=1 -test.v -test.timeout=10m` from `cmd/skgo`: exit **0**, one populated RUN/PASS, **8.15s**, no skips. This independently supplies a later pass for the adjacent fixture that initially failed while archiving a growing Gimbal log.

5. Independently checked the fresh outer cache now contains the actual `github.com/goccy/go-yaml@v1.19.2` zip. Its ziphash equals the pinned root go.sum value `h1:PmFC1S6h8ljIz6gMRBopkjP1TVT7xuwrButHID66PoM=`. No provisional capability/native-qualification SKGo version appears in that outer cache. Inline assertions exited **0**.

6. After both tests, `git diff --check` exited **0**. `git diff --quiet HEAD -- go.mod go.sum go.work go.work.sum plugins cmd/skgo/native_plugins_test.go dev_bridge.go internal/hotreload` exited **0**. Root module files, plugin source, the actual native fixture and accepted lifecycle code remain byte-identical to HEAD. This independently supplies the later passing source-integrity check after the initial candidate's go.sum mutation.

## Boundary assessment

Preparation executes normal Go module downloading against copied root go.mod/go.sum with `GOWORK=off` and a temporary `-modfile`; new sums stay in that temporary module. It runs before setting the private consumer cache and before the read-only dependency proxy is exposed. The consumer still uses only the local provisional file proxy plus the local read-only dependency server, `GONOPROXY=none` and its own GOMODCACHE/GOPATH/GOBIN. The provisional host is still installed by version using `go install`, and real plugin sources are still staged by the actual builder. There is no change to external-network behavior after preparation. Excluding `.gimbal/` removes live operational metadata, not application source; it directly addresses the observed archive-size race.

The worklog records both material follow-up failures and their later passing checks, cache conditions, real command results and limits. The independent results above should be appended or linked there by the caller, since this reviewer is authorized to edit only this report.

## Limits

This is local darwin/arm64 execution, with a genuinely empty dependency/archive cache but an intentionally reused normal Go compilation cache. The selected test binary was compiled independently from the current source before the empty-cache run; no custom proof script, rewritten fixture or raw output file was added. No new public CI job or Linux execution is claimed. Workflow syntax/selection and the complete real offline consumer/plugin boundary are independently demonstrated; accepted lifecycle runtime suites were not replayed. Commit, push, merge, release and downstream delivery remain with the caller.

No material unmet requirement remains for this bounded task. **No findings.**
