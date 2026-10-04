# Declared Inputs delivery — acceptance follow-up

Target: `c512e83110b8eddf802265fca6b44f915deb2d02`, following integrated source review round 11. Product source was unchanged while these receipts ran.

Outcome: **material findings remain**. The exact-target feature and successful-install evidence is substantial, but the real no-write refusal control exposes a consumer-configuration defect. This result supersedes any inference that round 11's source-only outcome qualified delivery. No merge approval is given.

I applied adversarial-review independently, inspected the actual Go proof drivers, native command logs, retained result files, and the corresponding product source. I did not execute these tests myself or edit product/proof code. Whole Inputs and delivery scope remains authoritative; the finding limit did not limit inspection.

## Finding 1 — issue: native pnpm bootstrap writes the project before a known conflict is refused

The delivery design requires different patch bytes, incompatible Kit/source, malformed configuration and workspace conflicts to refuse before authored writes. In `internal/kitpatch/kitpatch.go`, `Configure` calls `prepare` before `checkInstalledIfPresent`, `checkPatchDestination` and `checkPatchMap`. `prepare` calls `getPatchedDependencies`; that launches native pnpm (`runPnpm`, including its version invocation) before reading the raw workspace map or checking the conflicting destination.

The assumption that these native reads cannot write the selected project is false. The independent public CLI control invokes:

```text
go tool skgo kit-patch --web <different-destination fixture>/web --apply
```

The frontend already contains authored `patches/skgo-kit-3.0.0-queue.patch` with different bytes. The command correctly reports refusal to overwrite those bytes, but first creates `web/pnpm-lock.yaml`. The before/after selected-tree oracle changes from five entries to six and the test fails. The new lockfile contains pnpm 12.9.1 package-manager dependency/bootstrap records; it is not the expected patch-install result and was absent before the refused action.

Evidence, relative to sibling `prerender-inputs-proof/`:

* `delivery/consumer-controls-c512e83.log:99–100`: actual CLI command followed by `different-destination authored file/directory count changed5=>6`.
* `delivery-fixtures/existing-upgrade-sdk-c512e83-controls/refusal-different-destination.log`: the expected conflict diagnostic, showing this was a real product refusal rather than an unrelated failing command.
* `delivery-fixtures/different-destination-sdk-c512e83-controls/web/pnpm-lock.yaml`: the unexpected package-manager-only lockfile.
* `deliveryproof/delivery_test.go`: `rejectWithoutWrites` compares the complete selected authored tree; the conflict fixture had no native preinstall that could have created the lockfile beforehand.

Impact: a supposedly refused configure operation mutates the consumer's authored project. This violates the explicit delivery contract even though SKGO did not directly write the lockfile. The earlier source pass missed native package-manager bootstrap as an effect of launching the getter.

Bounded correction: perform all available local-only path, raw YAML/shape, destination, installed-byte and patch-map conflict checks before invoking pnpm. Preserve the native effective-map comparison before any intended writes. Native/raw ambiguity refusal also needs its no-write behavior retained; moving only the destination check is not sufficient for the other local preflight cases. Any claim that a remaining native read is non-mutating needs actual source/behavior support, not a guessed package-manager flag. Re-run the original complete-tree refusal controls unchanged against the corrected checkpoint. Do not preinstall the target, ignore the new lockfile, or delete it after refusal to make the oracle pass.

## Exact-target evidence now available

The following were inspected as results for `c512e83`, rather than borrowed from earlier implementation checkpoints:

* `product-final-c512e83.log`: **PASS, 209.786s**, fourteen top-level product cases and seven malformed/fatal subcases, no skips. The driver performs native packed-adapter installs into independent fixture roots. It asserts literal atlas/beacon/Money/noargument artifacts; producer/body counts and no generation/runtime execution; duplicate consumption; compiled Go HTTP/Goja SSR and existing static/SSR behavior; true JavaScript output; body-error origin/hooks and opaque no-hook fallback; native producer and crawler failures; owner and actual outer-VP signals; inherited output pipes; and real compiler failure. Native programmatic rejection receipts record zero owned children, absent temp directories and no later I/O. The crawler receipt preserves `Error: Prerendering failed`. The original minimal noargument and same-export cases pass in 4.80s and 4.59s respectively.
* `delivery/consumer-c512e83.log`, fresh-consumer subtests: **PASS, 73.35s** total. TypeScript uses the default Storybook scaffold branch; JavaScript uses the selected branch. Each runs the real archived `Create/finish` path, native package install, public `--check`, generation/build and compiled HTTP/SSR. Each then deletes dependencies, relocates authored files, installs frozen into a fresh store, and repeats build/runtime assertions. Literal producer/body counts and native artifact payloads are asserted, as is an actual inert helper invocation in Goja. The same combined log contains earlier setup failures in other tests and must not be described as an entirely passing suite.
* `delivery/consumer-controls-c512e83.log`: existing consumer upgrade/apply/check/idempotence/frozen relocation **PASS, 22.91s**. Its native/raw preservation assertions include unrelated relative and absolute patch paths and valid YAML alias/flow forms. Unsupported installed version and altered source refuse with unchanged authored trees; removal of only the owned correction restores the named stock guard failure. Implicit and explicit parent-workspace refusal also pass unchanged-tree checks. The later conflicting-destination case is the material failure above, and aborts that test before its remaining malformed-map cases can be claimed green.
* `delivery/native-worker-guard-c512e83.log`: **PASS, 5.28s**. The exact generated `$skgoRemoteInputs<string>("src/lib/fixture.remote.ts", "catalog")` factory runs in a real Kit worker while a generated Go load is live and ignores TERM. The native install is stock Kit and HTTP 500 policy is ignore. Its rejection names the actual stock `dbe8bb…6d5` and expected corrected `400bf3…a0f5` hashes. `delivery-fixtures/guard-native-worker-sdk-c512e83-controls-exact-factory/rejection.json` records group 48711, `childrenAtRejection:0`, `tempDirAtRejection:false`, and `lateIO:0`. Go Inputs/body receipts are absent. This establishes the current metadata resolution and fatal/drain path, not merely an earlier A checkpoint.
* `delivery/consumer-c512e83.log`, corrected native delayed-input case: **PASS, 3.14s**. The retained stock native negative and separately retained universal import-only positive remain independent controls.

The original proof failures are retained honestly: the initial combined consumer run stopped some cases during SDK replacement module-graph setup or because the fixture authored an invalid pnpm setting; the first current guard wrapper referred to the old generated helper alias and failed before creating its Go load. The corrected runs used native Go tidy, a valid workspace sentinel and the exact generated helper expression. Those setup failures are not counted as product successes, and their correction does not justify weakening the unrelated complete-tree refusal failure.

## Remaining qualification boundary

The parent reports root `just build`, vet and tests passing on this product source; I have not independently executed those commands. Protected exact-head CI and ordinary application qualification remain pending, as does review of the correction for Finding 1 and its unchanged controls.

The fresh path proves actual `Create/finish` through the existing addonStage seam, using packed bytes and an explicit immutable prerelease SDK replacement. It is not an actual public `skgo new` CLI or released Go-tag/npm-registry qualification. The existing-consumer path does exercise the actual public `kit-patch` CLI. No successful fixture is evidence that the package has already been released.

The seven-group and consumer positives remain valuable evidence. They do not cancel the concrete refusal-side-effect failure.
