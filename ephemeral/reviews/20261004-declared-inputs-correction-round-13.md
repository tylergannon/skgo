# Declared Inputs delivery — correction acceptance review

Target: `13d5e6e05d7f37f8f99cfed324af173d1dac2e04`, following immutable integrated source round 11 and acceptance round 12.

Outcome: **no findings**. The round 12 refusal-side-effect finding is closed by the bounded source correction and actual unchanged consumer controls. No material source or demonstrated feature/delivery defect remains in this review. Protected exact-head CI, ordinary application qualification and actual released-package qualification remain separate; this report does not claim those have completed.

## Scope and evidence method

I applied adversarial-review independently. The whole seven-group Inputs assignment, canonical native queue correction, Go/npm identity, fresh/existing consumer delivery, and earlier findings remain the review scope. I read the entire `c512e83..13d5e6e` diff, surrounding helper/tests, actual independent proof drivers and retained native/CLI/runtime receipts. I did not delegate review or edit product/test sources.

The only product difference from `c512e83` is `internal/kitpatch/kitpatch.go`; its tests and worklog also changed. Go Inputs runtime, generator, adapter JavaScript, native patch/metadata and npm assets are unchanged. Therefore the whole-feature evidence remains explicitly attributed to `c512e83`, while the configuration correction and consumer reruns are attributed to `13d5e6e`. No earlier log has been relabeled as a new-target run.

## Closed finding: preflight launches no bootstrapping native read

At `c512e83`, independent public CLI controls demonstrated four manifestations of the same cause: conflicting patch destination, non-string patch key, non-string patch value and native environment/raw-map disagreement each refused correctly but created a package-manager bootstrap `pnpm-lock.yaml` first. Round 12 and the original failed logs remain immutable.

The corrected ordering is bounded and complete:

* `prepare` reads local metadata, physical frontend/package information and raw YAML without launching a process.
* Configure checks installed source, patch destination and raw patch-map conflicts before native validation. Verify requires the direct pin, canonical patch/config and corrected installed bytes before native validation.
* `validateNativePatchedDependencies` retains native effective-map/raw-map agreement and additionally checks the second raw read still agrees with the prepared map. `equalStringMap` requires actual key presence, so missing keys with empty values cannot compare equal.
* Every native helper invocation prefixes `--pm-on-fail=ignore`, including the preliminary `--version`. The exact `12.9.1` output requirement remains. Configuration set remains native pnpm's operation; Go still does not emit YAML or author a lockfile.
* The owned entry must retain the exact canonical portable relative path. Parent/global workspace refusal, symlink checks, incompatible-source refusal and unrelated raw relative/absolute values remain intact.

The flag is supported current behavior, not a guessed legacy option. [pnpm's official CLI settings](https://pnpm.io/settings/cli#pmonfail) describe `pmOnFail: ignore` as skipping package-manager version management and identify the removed legacy settings it replaces. I inspected the installed 12.9.1 native launcher/help and ran an authorized disposable Go control through the actual VitePlus pnpm shim. Both version and getter commands retained the complete lockless selected tree; the version stayed exactly `12.9.1`. `PNPM_CONFIG_PATCHED_DEPENDENCIES` still produced a distinct effective map, so the flag does not suppress the ambiguity check. The retained witness is `ephemeral/experiments/20261004-declared-inputs-consumers/pnpm-read-witness/` (original owned run: `task-8/tmp/pnpm-read-review/`).

The independent proof agent's separate `delivery/native-getter-ignore-c512e83.log` establishes the negative as well: unflagged version/getter calls each create a lockfile in fresh projects, while both flagged variants leave the complete tree unchanged, including dependency directories. This isolates the correction's mechanism from the public CLI tests.

## Actual corrected consumer acceptance

I inspected `prerender-inputs-proof/delivery/consumer-13d5e6e.log` and its Go driver, not only the reported exit code. The complete selected run is **PASS, 94.483s**, with no skipped cases:

| Actual case | Result |
| --- | --- |
| Fresh packed TS, default Storybook branch, then relocated frozen reinstall | PASS, 30.82s |
| Fresh packed JS, selected Storybook branch, then relocated frozen reinstall | PASS, 36.18s |
| Existing stock consumer upgrade, public apply/check, native install, idempotence, relocated frozen reinstall | PASS, 25.19s |
| Ten public CLI no-write refusals | PASS, 1.38s |
| Unsupported installed version and altered source | PASS, 0.67s |

The ten refusal subcases are implicit parent workspace, explicit parent workspace, different destination, incompatible Kit patch, malformed YAML, duplicate YAML, non-string key, non-string value, complex key and environment ambiguity. The original whole-authored-tree hash/path assertions remain. I read the actual four previously failing diagnostics; each now reports its intended conflict/type/ambiguity error and passes the unchanged before/after comparison. No target preinstall, ignored lockfile or post-refusal deletion was substituted for the required behavior.

The ambiguity case first demonstrates the native override independently: the authored `unrelated@1.0.0` path becomes the separately authored `other@1.0.0` effective key/path under the environment setting. That flagged read itself leaves the authored tree unchanged. The actual public Go CLI then rejects the disagreement and again leaves the tree unchanged. It is not a mock-only mismatch or an ignored environment variable.

Fresh and existing positive paths use actual npm-packed artifacts and immutable exported SDK source. The driver checks that installed adapter/Kit/VP packages resolve into owned native package trees rather than the product checkout. It verifies the canonical queue hash, runs public `kit-patch --check`, actual generation/build and compiled Go HTTP/SSR with literal Inputs artifacts, producer/body counts and the real inert helper invocation. Deleting dependencies, relocating authored files, and frozen installation into new stores precede the repeated build/runtime assertions. Existing unrelated relative and absolute patch values, alias/flow syntax and authored sentinels remain asserted. The configured-but-uninstalled check and initial stock failure are real public command/build controls.

## Whole-feature evidence retained with its correct scope

Round 12 already records the inspected `product-final-c512e83.log`: fourteen top-level actual product cases and seven malformed/fatal subcases, **PASS 209.786s**, including the original minimal noargument and same-export cases. Its literal artifacts, Money, deduplication/counts, no generation/runtime producer calls, native structured/unknown error behavior, compiled HTTP/Goja SSR, true JS, native producer/crawler failures, owner/outer-VP signals, pipe inheritance and compiler failure are evidence for the unchanged runtime and adapter source in this target.

The actual current-adapter `native-worker-guard-c512e83.log` passes with real stock/corrected hash diagnostics under ignore500 and a live TERM-ignoring Go load. Its outer rejection receipt has zero owned children, absent private directory and zero late I/O. The retained cleanup audit separately confirms 157 command/body groups and seven private directories absent. The corrected-native delayed-producer positive, original stock negative, universal import-only witnesses, maintained queue overlap/recursive-addition controls, strict undeclared-ID negative and TS/JS identifier collision controls remain intact.

I inspected the retained root `just build`, vet and test outputs for `c512e83`; the parent additionally reports targeted kitpatch/public-command/newapp tests and root vet passing for `13d5e6e`. I did not independently rerun those expensive suites. Earlier rounds retain the precise source/receipt analysis for API signatures, reserved keys, transport graph/class identity, error provenance, permanent compilation ownership, failure barriers, cleanup retry, successful cleanup, package fingerprinting and all seven acceptance groups. No correction here changes those boundaries.

## Qualification boundary

This closes the last material finding identified in rounds 11–12. The consumer proof is prerelease delivery proof: fresh applications exercise actual `Create/finish` via the existing addonStage seam using unpacked packed add-on bytes and an explicit immutable SDK replacement. It is not a claim that public `skgo new` was invoked against already released packages. Existing consumers do exercise the real public `kit-patch` CLI.

After the final evidence commit, protected exact-head CI and ordinary application qualification still need their own results. Actual public Go-tag/npm-registry qualification remains required after an authorized release before claiming released consumer delivery. No source or test finding remains that calls for further architecture changes or weakened acceptance checks.
