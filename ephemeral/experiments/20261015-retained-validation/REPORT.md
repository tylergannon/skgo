# Independent retained preparation qualification

The new whole-application retained acquisition path passed independent handler and input-admission controls. It is not the older root-only137ms experiment: production CompileApplication still runs its classifier, lowering, native compiler, object/profile hashing, and real HostView installation. Only package acquisition is overlaid. No production source was edited by this validator.

## What ran

Preserved `compile.go.txt` and `loader_test.go.txt` snapshot the implementer's exact seam tested. `overlay.json` adds validation-owned tests without replacing the implementer's files. All Go runs used `-count=1`; raw outputs are retained.

- `handler-positive.log`: `TestRetainedAdmissionContract` PASS8.07s. This is a validation-owned augmentation of the existing production generated handler contract: real pinned Kit build, actual generated Go remotes/loads/endpoints/actions, real native child and pooled Go SSR, private HostView with pinned compiler/dependency identity, actual installation, literal HTTP expectations and unchanged production build check. It preserved the original real-handler assertions and forged host identity/dependency/profile/content refusals. Added import/constant/type structural changes all required CompleteBuild before install. A caller package existed before warm preparation with only a type reference; editing its body to call ExportedPrice was rejected as an uninstrumented caller before candidate.o existed. This qualifies warm caller-body invalidation, not only discovering a new package.
- `dependency-positive.log`: five warmed loader fixtures independently changed an imported constant, imported type, import set, new Go file membership, or embedded resource bytes. Every one selected CompleteBuild. No patched object was installed after these incompatible inputs. The embedded and added-file cases were reported as source gaps to the implementer; the actual tested source includes explicit checks for both.
- `external-caller.log`: a separate local replacement module outside the main app's ./... roots initially used only target.Order. After warm acquisition, its body changed to target.Value(). Retained acquisition typed that reverse caller and Classify required CompleteBuild. Same edited snapshot with a deliberately narrowed target-only application graph was incorrectly admitted. This directly demonstrates that retained complete reverse caller coverage is load-bearing.
- `dependency-guard-omitted.log`: with only dependency rejection guards removed in validation-owned `loader-without-dependency-admission.go.txt`, all five original independently supplied rejection expectations FAILED, because changed inputs were admitted with nil error. This is an executed broken control, not a hypothetical source audit. No unsafe candidate was installed. The unmodified snapshot's positive log is the restoring comparison; no source was changed after its positive execution.
- `stale-project.log`: existing production `TestRetainedProjectStaleGeneration` passed uncached in0.35s. A controlled blocked preparation was superseded; it returned ErrObsolete and wrote no stale generated output, then current generation committed the independently expected new field. This verifies the existing Project boundary separately. The qualification loader itself does not own a revision/currentness scheduler, so it does not prove obsolete native candidates cannot publish in a future joined supervisor.

## Boundaries and cost

The independently run contract loaded9 application roots /9 typed roots /266 graph packages /5 affected reverse-closure packages, reflecting the extra validation caller. Logs show2 metadata loads and subsequent hits,0 fallback. This counters the concern that the fast number silently omits whole-application callers. Local replacement coverage has an independent positive/negative control.

The implementer's separately coordinated timing run measured whole-retained cold548.236ms and warm168.108/150.521ms plus3.786/3.792ms install. Read the implementer's finalized report/raw logs for exact timing identities and stage definitions. This validator avoided concurrent heavy compilation during that run; the8.07s contract duration is total test setup/assertions, not per-edit latency. The older137/138ms root-only samples and later approximately1s full-analysis browser path remain historical, distinct measurements.

A dependency/import/type/membership/resource change currently returns CompleteBuild; these tests do not demonstrate that an automatic supervisor rebuilds and restarts it successfully. Real HostView runtime mismatched-profile checks were exercised, but no changed dependency was admitted to the old host. The implementation is a bounded experiment with explicit captured-input ownership; it does not yet prove atomic project capture during concurrent filesystem mutation, full application admission for every program, resource bounds, or stale native-publication prevention. Arbitrary initialized Go state/unloading are not gates.

Browser/native save-to-visible joining is owned by the implementer and separate browser observer. This report's handler contract proves server behavior; it does not assert a browser UI. Do not call it an end-to-end browser timing result.

## Reproduce

From this worktree root:

```text
go test -overlay ephemeral/experiments/20261015-retained-validation/overlay.json -tags 'skgo_retained_validation skgo_retained_qualification' ./internal/hotreload -run '^TestRetainedAdmissionContract$' -count=1 -v -timeout 3m
go test -overlay ephemeral/experiments/20261015-retained-validation/overlay.json -tags 'skgo_retained_validation skgo_retained_qualification' ./internal/hotreload -run '^TestRetained(CapturedDependencyInputs|ExternalReplacementCaller)$' -count=1 -v -timeout 2m
go test -overlay ephemeral/experiments/20261015-retained-validation/broken-overlay.json -tags 'skgo_retained_validation skgo_retained_qualification' ./internal/hotreload -run '^TestRetainedCapturedDependencyInputs$' -count=1 -v -timeout 2m
```

The broken control must exit nonzero with all five rejection expectations failing. Overlays contain current absolute worktree paths; rebase them before moving the experiment. Native host processes were joined/stopped by test cleanup. No fixture listeners or detached native hosts remain owned by this validation lane.
