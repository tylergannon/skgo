# Remote command and form caller params

## Current continuation instructions — 2026-10-06

This chat (`01a11328-2836-7ee3-9276-dab87a186ec6`) owns direct orchestration,
without Gimbal. Work in the preserved `codex/remote-function-params` worktree at
`/Users/tyler/.codex/worktrees/remote-function-params/skgo`. Main integration
and the previous implementation/independent validation are complete at
`7734653`; `8e0d62a` adds investigation notes. PR #258's prerequisite is already
integrated. Do not restart that integration or historical Gimbal runs.

The user has reopened the public Go API. This revision prepares the next
implementation; it does not claim that the new API has been implemented or
validated. Preserve the working caller routing, matching, decoding, native and
enhanced forms, isolation, drift validation, and precise typed loads. Previous
proof covers the previous API, not this revision.

The orchestrator assigns bounded coding missions and owns independent QA.
Each builder implements its assigned capability, runs focused developer checks,
and returns changes, results, and remaining gaps. **Builders do not launch
reviewers, validators, coordinating agents, or workflows.** Repeat that boundary
in each assignment. The parent dispatches the independent verifier afterward
and manages concrete repairs. Source-changing checks and mutations run
exclusively. Task instructions belong here and in assignments, never AGENTS.md.
No feature PR, merge, release, site-pin change, or deployment is authorized.

## Revised public API

Commands and forms receive `context.Context` first, an explicitly instantiated
`skgo.RequestEvent[params.Params]` second, and their explicit input afterward.
Commands may omit input; forms require it. Results retain their existing shape.
For example:

```go
func save(
    ctx context.Context,
    event skgo.RequestEvent[params.Params],
    input SaveInput,
) (Receipt, error)
```

`skgo.RequestEvent[P]` is the one reusable generic event implementation.
`params.Params` is the one generated concrete app-wide superset type; each
request gets its own value. The package qualifier `skgo` names the existing
framework type. The user's `AppWideParams` was illustrative: retaining the
existing `params.Params` avoids an unrelated rename. Do not replace the generic
event with an application-specific event implementation, and do not hide its
type argument in the canonical authored examples through `params.RequestEvent`.
Ordinary Go aliases do not create a different type; signature validation should
check type identity, not source spelling.

The supplied ctx must retain request cancellation, deadlines, values, and
request-scoped cookie/refresh helpers. Passing it to a query, including a direct
nested Go call, must not reveal the command/form caller's params, URL, or route.
The explicit event supplies that caller state. The existing `event.Context()`
restriction is a compatibility boundary to preserve, not permission to forward
an unrestricted dispatcher context. Context and event must belong to the same
request. Query/batch/live/prerender signatures and route-local load APIs remain
unchanged.

`event.Params.Number()` returns a generated per-key marker interface. Its
concrete alternatives wrap the original declared Go type in `Value`; truly nil
means absence. Original methods, named type identity, accepted falsey values,
and present nil payloads must survive. There is no outer Optional and no public
`any`/map escape hatch. This is Go's conventional closed-interface pattern, not
an exhaustive-switch guarantee or a security boundary against hostile embedding.

Readable switch cases use the accepted parameter/type naming pattern:

```go
switch number := event.Params.Number().(type) {
case params.NumberParam_String:
    return number.Value, nil
case params.NumberParam_Int:
    return strconv.Itoa(number.Value), nil
case params.NumberParam_OrderNumber:
    return number.Value.Label(), nil
case nil:
    return "absent", nil
}
return "", fmt.Errorf("unsupported number variant")
```

The ordinary named case retains the actual domain name `OrderNumber`; it is not
arbitrarily shortened to `Order`. Literal `case string`/`case int` cannot satisfy
a marker interface, and those variables would have no Value field. The prior
compiler probes established that boundary. Do not silently weaken the return
type to `any` to imitate that syntax. A default must not treat unknown variants
as absence.

## Accepted variant naming

Use `<Key>Param_<Type>`: `NumberParam_String`, `NumberParam_Int`, and
`NumberParam_OrderNumber`. If distinct types collide, qualify with the declaring
package: `sales.OrderNumber` and `legacy.OrderNumber` become
`NumberParam_SalesOrderNumber` and `NumberParam_LegacyOrderNumber`.

Adding a real collision may rename the affected alternatives; that tradeoff is
accepted. Names remain deterministic for the same inputs. Preserve actual Go
type identity and alias deduplication. If readable qualification still collides,
use a deterministic identity suffix only for those colliding alternatives.
Do not impose hashes on ordinary names or add user naming configuration, a
migration framework, or a naming ledger. The old unconditional name-stability
requirement and collision-as-user-error proposal are superseded.

## Presence and the matcher type domain

Presence comes from the selected route's parameter membership and matcher acceptance bool, never truthiness. Accepted zero, false and empty string are present wrappers. Missing optional segments and absent names return truly nil interfaces. Generation/runtime construction returns concrete non-pointer wrappers for presence, never typed-nil wrapper pointers. The same key may be present with a different variant on another route or after candidate rejection/fallback.

The wrapper's Value retains the declared original Go type and its methods; do not flatten it by underlying kind. Distinguish the two language contracts explicitly. Kit 3.0.0's JavaScript `ParamValue` is string | number | boolean | bigint and `run_matcher` rejects other accepted JS results, including null. The paired JS/TS matcher must obey that rule. The accepted Go typed-load API already enriches a matching path into arbitrary original Go values: `TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation` in `internal/gen/load_params_test.go:37–155` changes Order to `RevisedOrder struct { Number int64 }`, refreshes accessors, then compiles and runs real data-handler tests proving `Text()` returns literal `Revised order #42`. This is demonstrated Go representation behavior, not a claim that Kit permits JS object matcher results.

Preserve that existing Go contract in the superset for every caller route, including load-only routes. Named structs belong in wrappers too; they are not excluded, flattened to numbers or forced through a scalar-only fixture rewrite. The scalar-only Go diagnostic proposal in the previous revision is superseded. Route candidate/optional/fallback behavior follows Kit, and paired matchers must agree on route acceptance and intended domain meaning for independently specified paths. Go values are server-side application representations; do not serialize them into browser-authoritative params or pretend their Go type is a JS matcher output. This Go enrichment is an explicit existing skgo extension at the language boundary, not a new JavaScript matcher domain.

The supported generation domain is compiler-known T legally expressible and importable in the shared package, preserving current matcher reading/emission rather than adding a new type interpreter. Use original named types, scalar/struct types, aliases, and legally nameable generic instantiations. Pointer/nil-capable representations are permitted when the compiler/emitter can preserve their type. An accepted nil pointer/slice/map/function/channel or interface result is a PRESENT concrete wrapper with nil Value; absence is only a missing key after matching. A declared interface result (including nil or an interface containing a typed nil) selects the wrapper for its declared T, not its dynamic payload type. Construction must handle accepted nil interface payloads using declared metadata and membership, never an unchecked nil `. (T)` assertion or a typed-nil wrapper. Methods on a nil Value follow the original domain type's contract. No deep-copy promise for application-owned mutable values; framework containers and match results must remain request-local.

Prove actual supported representations with generated compilation/runtime fixtures, including the already accepted named struct and at least an unnamed pointer `*domain.OrderRef` to an exported named struct accepted as nil (not a defined pointer type `type P *T`, which cannot declare receiver methods). Scope additional unnamed/function/channel/interface tests to legal compiler-nameable results, and report concrete unresolved generator obstacles instead of claiming support from a signature check alone. Inaccessible named types/type arguments or structural members, illegal internal imports, caller-route-owned types and cycles get source-located diagnostics with the leaf-package remedy. Such diagnostics are accessibility/ownership limits, not a new blanket non-primitive matcher ban.
Matcher false follows Kit's routing algorithm: it normally rejects the candidate, but a chained optional `[[name=matcher]]` may leave that key absent and carry its segment forward into following params. Preserve the `buffered` behavior in Kit exec and current execMatchedParams, rather than treating every false as whole-route rejection.
## Generation, identity and package ownership

Generate the shared definitions into a leaf sibling package `<generator-output>/params` (for this example `example/internal/skgo/params`), not into the bindings package or any caller route. Remotes import it directly. Bindings may import both remotes/routes and params; params imports only skgo and packages needed to name original domain types. It must not import a caller route or a remote package. Existing matcher/domain packages must form an acyclic dependency graph beneath params; diagnose violations and explain the leaf domain-package remedy. A domain type located in a caller route cannot be imported as a shortcut. No new migration framework or automatic source rewriting.

Collect route metadata from every Kit caller route, including routes with no Go load and routes with no hooks. Reuse the load foundation's matcher reading/type information, but do not use its set of load directories as the universe. Plain segments produce string alternatives; optionality produces absence, not a second alternative. Runtime variant selection is keyed by parameter name plus the selected route's declared result type/matcher metadata. Candidate conversion must finish successfully before exposing its values; rejected candidates must not leave partial state.

Deduplicate using Go type identity (`types.Identical` with aliases unaliased), not Type.String, matcher name or underlying kind. Aliases of the same original type share one variant; distinct defined types with identical underlying types, including identically named types in different packages, remain distinct. Named generic instantiations include their arguments; pointer T differs from T and legally expressible unnamed types deduplicate by actual Go structural identity. Multiple matchers returning the same T share the variant while retaining their independent acceptance/conversion behavior.

Public naming follows the accepted policy above. Fixtures
must establish readable ordinary cases, distinct identities for colliding Go
types and parameter keys, deterministic regeneration, and the documented
behavior when a newly added type collides. Keep private markers/storage and
helper declarations collision-safe without exposing their machinery in every
application switch case.

Bootstrap and regeneration precede full application handler type checking. Independently parse Kit route metadata and load matcher/domain compiler information, then write/refresh the shared params and precise load params before loading remote bodies which import them. First generation must work without the generated package. Adding a variant must refresh it even if an old handler body has a stale reference or changed matcher type and currently fails compilation. Once refreshed, type-check bodies and report actual authored errors. Read-only generation/check detects missing/stale output without rewriting. Do not use stale generated type information as the source of truth or require placeholder hand-authored structs. Matcher/domain imports of generated params themselves are dependency cycles to diagnose, not bootstrap sources.

## Kit mapping and runtime seams

The active worktree's installed Kit package is verified as 3.0.0, per
`ephemeral/sveltekit-current/SKILL.md`. Builders and the independent verifier
must inspect that pin themselves; these references are orientation.

Kit's `src/runtime/client/remote-functions/shared.svelte.js:get_remote_request_headers` sends the current or navigating-to pathname/search independently of explicit arguments. Enhanced forms carry this context too. `src/runtime/server/respond.js:167–175` skips caller route resolution only when pathname is absent; event.params starts empty; `:371–381` calls find_route and assigns route/params before dispatching the known remote at `:634–635`. An unmatched caller route leaves params empty and does not make a known remote unknown. `src/utils/routing.js` checks candidates and matcher rejection permits fallback. The remote definition directory does not determine caller params.

Kit decodes pathnames through `src/utils/url.js:decode_pathname`, splitting on `%25` and applying decodeURI to each piece; parameter extraction then uses decodeURIComponent (`src/utils/routing.js`). `respond.js:296–301` catches malformed decoding and resolve returns 400 before remote dispatch (`:601–610`). A single generic PathUnescape is not an adequate substitute: preserve encoded slash and percent handling. Literal handler parity must demonstrate the result.

Kit's `src/runtime/app/server/remote/shared.js:derive_remote_function_event` preserves command/form context, disallows setHeaders and enforces cookie limits. Under is_in_remote_query it makes url/params/route throw (`:115–125`), including nested queries. Therefore the application-wide params event is only for command/form. Preserve query restrictions across SSR, direct nested calls, refresh, batch/live and dev-prerender constructors. No load tracking is needed for remote Params.

Current implementation already constructs the generic event and shared Params.
The generator recognizes command/form event-first signatures and emits calls
without an explicit ctx. The next implementation must accept the revised
context-first signatures, validate the app-wide type in the event argument,
and pass both arguments correctly through generated dispatch. Update authored
consumers, diagnostics, generation fixtures and marker documentation together.
Event-only and ctx-only authored command/form shapes should receive the new
source-located signature guidance; do not add an undocumented alternate mode.
Low-level runtime callback adapters may remain context-based.

Explicitly settle all inherited raw APIs: for command/form events, both raw `Param(name)` and `Params() map[string]string` are forbidden legacy access (including `event.Event.Param`, `event.Event.Params`, and `EventFrom(event.Context()).Param/Params`), with a clear error/panic consistent with the remote-property restriction mechanism. Their supported accessor is `event.Params.<name>()`. A shadowing method alone is insufficient because Context() and the embedded Event are reachable. Keep raw load/middleware behavior outside the remote context unchanged. Query-derived contexts must forbid params, URL and route access and must not retain a usable command superset through existing context helpers. Do not expose raw strings as an inconsistent second command/form params API. This is a deliberate boundary within the new explicit remote-event API, to be reviewed against existing usages before implementation.

## Definition of done: executable contracts

Use ordinary Go tests, existing production/dev BDD, and independent Claude Opus QA. No new proof framework, harness, ledger, acceptance runner or feature-specific site page. Expectations are literal fixtures independent of the implementation. A skip is an unmet check.

1. **Generation and compilation.** Actually generate Go and compile/run consumers of the shared Params. A remote imports only the shared params package and domain types, without any caller package import. At least one shared `id` is a named numeric matcher value with a method on one route, plain string on another, absent on a third/optional omission. Type-switch receipts assert the exact variant and Value/method result for each, including accepted zero/false/empty. Generate a named struct variant retaining fields and methods (the accepted RevisedOrder load test must remain intact), and an unnamed `*domain.OrderRef` pointer accepted as nil whose pointer wrapper remains present. Where supported, accepted nil interface payloads retain the declared interface variant. Unsupported accessibility/ownership cases fail with source-located diagnostics; JS/TS matchers still obey Kit primitive results. Negative compilation proves unrelated plain int/string/domain values cannot be assigned directly to the variant interface. Tests cover named-type vs alias deduplication, identical underlying values in different packages, import-name and public-name collisions, legally nameable generic and structural identities, with declared interface identity where supported. First generation and stale-handler regeneration both work. Add a noncolliding third alternative in regeneration: old public names remain stable, a consumer handles it explicitly, and an old consumer which handles nil but rejects unknown variants does not mistake it for absence. Do not claim all missing switch cases are detected by Go.
2. **Real command/enhanced-form handler entries.** Call the same command and form from multiple caller shapes with distinct literal explicit input. Same-name/different-type receipts prove selected route conversion, without context retrieval for authored params or inference from remote directory. Include hookless, Loads-less, matched-loadless, optional omission and candidate rejection followed by fallback where the SAME key changes variant. Headerless generated Go form-client calls and unmatched caller URLs execute a known remote with every relevant getter nil; unknown remote identity remains a separate 404. No serialized browser params are authoritative. Encoded Unicode/segments/percent/slash use literal Kit-derived expectations for remote, document and data paths, never values read from one path as the oracle for another. Include `/items/%2525` -> `%25` and `/docs/a%2Fb` -> single-segment value `a/b`, plus malformed caller path 400 before handler invocation. Preserve the implemented shared decoding correction for document/data as well as remotes; do not regress precise load API/tracking semantics. Include Kit-derived chained optional rejection fixtures `/[[lang=Lang]]/[[id]]` from `/abc` (Lang matcher is actually invoked and rejects captured `abc`; Lang getter nil, ID string wrapper Value `abc`) and/or `/[[lang=Lang]]/[...rest]` from `/abc/def` (Lang nil; Rest string wrapper Value `abc/def`). Assert matcher invocation explicitly. The earlier `/[[lang=Lang]]/[id]` from `/abc` fixture exercises regex backtracking without calling Lang and is superseded as carry-forward proof.
3. **Native forms.** Literal urlencoded HTTP POSTs to `?/remote=<id>` on multiple caller pages, including keyed `form.for` identity, assert variant/value receipts in the returned document in production and dev SSR. These supplement enhanced endpoint tests and JavaScript-off browser submission. Preserve form codecs/validation/retained fields and generated Go client behavior. Adding optional caller URL support to that Go client is a separate decision.
4. **Isolation and existing semantics.** Alternating callers and concurrent overlapping requests have independently fixed receipts; no shared storage/cache/hook state leaks across requests or fallback candidates. Embedded/context-derived raw Param and Params are forbidden consistently in command/form; Params getters remain correct. Queries called directly inside commands, SSR queries and Refresh/RefreshRequested queries cannot observe caller Params, URL or route. Cover batch/live/dev-prerender entry paths affected by common constructors. Preserve existing cookies, CSRF, header limits, redirects, cancellation, refresh/single-flight handling and codecs.
   **Accepted nil interface at the load boundary:** Preserve the implemented LoadParamValue[T]/OptionalLoadParamValue fix for interface-typed matchers returning `(nil, true)`, with unchanged accessor tracking. A declared-type-aware branch must return a present nil interface value only for the accepted nil-interface case; map membership determines presence, and mismatched non-interface nil must not silently become an unrelated zero value. Real handler fixtures for required and optional interface params prove the load runs, its optional pointer-to-interface is non-nil with nil payload when present, actual omission remains absent, and shared remote wrappers on the same caller routes are present with nil Value. Literal nil-interface fixture: matcher MaybeRef returns declared `domain.Ref` (interface with Label method), `(nil, true)` for `none`, a concrete Ref with Label `Ref #42` for `42`, and rejection otherwise. Required `/nil/[id=MaybeRef]` and optional `/nil-optional/[[id=MaybeRef]]` loads receive it. Real data-handler requests `/nil/none/__data.json` and `/nil-optional/none/__data.json` return HTTP 200 with exact receipt `load:present:nil`; `/nil-optional/__data.json` returns `load:absent`; `/nil/42/__data.json` returns `load:present:Ref #42`. Assert the optional getter's pointer is non-nil with nil interface payload for `none`, and nil only on actual omission. Commands/forms from the same caller routes return exact `remote:present:nil`, `remote:absent`, and `remote:present:Ref #42` receipts through the declared Ref wrapper. Unnamed pointer fixture uses `*domain.OrderRef` and distinguishes exact `pointer:present:nil` from `pointer:absent`. Tests include positive handler execution, not merely lack of output. Retain the existing RevisedOrder fixture unchanged. Independent QA must establish removing this nil-boundary fix makes those load-handler assertions fail. This targeted consistency fix is included explicitly; do not invent a new load API or general conversion framework.

   **Compiled/manifest drift:** Validate the actual served manifest snapshot against the generated complete route/key/matcher metadata before selecting any candidate, in production and dev. A new unknown key, matcher or missing constructor must fail loudly before invoking a known remote, rather than produce nil or reject that candidate into a different fallback. Use an explicit startup drift error or visible request failure until regeneration/rebuild; tests assert the error or literal failing response and zero handler calls. Cover frontend rebuilt without generation, transient dev manifest ahead of the Go rebuild, and an unknown matcher on the first candidate with an otherwise valid fallback. Selected-route-only validation is insufficient. This is an extension of existing drift checks, not a new framework.

5. **Kit client behavior.** Extend existing example BDD with actual client navigation among numeric, string and absent caller shapes using the same command and form. Literal receipts include both variant and value, with positive visible page content. Enhanced JS and JS-off forms work in production and dev. Ordinary HTTP-only claims belong in Go handler tests. No new site-specific remote demo page; future site work extends the existing shared /demo after the library feature exists.
6. **Independent QA.** Claude Opus runs the existing required Go/build/check workflows and production/dev BDD with no silent skips, personally inspects real page behavior, and establishes load-bearing assertions with deliberate restored mutations. Required mutations catch wrong variant, erased named type/method (including a compilation failure when appropriate), dropped caller state, falsey treated as absent, wrong route/fallback, rejection of a captured chained optional as whole-route rejection, and inter-request leakage. Also establish query restriction, native form, and the required/optional accepted nil-interface load boundary claims are load-bearing. Reuse captured expensive-suite results; repeat only for changed behavior, failure reconciliation or necessary mutation proof. Passing workflow status alone is not proof.

## Development sequence

1. **Preparation complete.** The read-only preparation review is complete and
   the user has settled naming as recorded above. Do not reopen that decision
   or repeat the preparation review.
2. **Implement the coherent API revision.** One coding agent owns the generated
   public surface, signature acceptance, dispatch, authored consumers and their
   immediate fixtures as one mission. Success means developers can write the
   explicit generic context-first signature and readable switches above while
   all preserved behavioral contracts remain true. The agent chooses its
   implementation path and hands back focused proof; it launches no validators.
3. **Independently validate and repair.** The parent dispatches Claude Opus on
   stable source. In addition to preserved contracts, prove explicit ctx
   cancellation/deadline/value propagation, cookie/refresh state, and nested
   query restrictions through real generated handlers; prove correct argument
   positions with both no-input commands and input-bearing commands/forms.
   Compile generated consumers with the explicit generic spelling and prove
   wrong app-wide type arguments and obsolete signatures are diagnosed.
   Naming fixtures exercise the settled collision policy. Preserve keyed-form
   validation feedback and retained fields after client navigation, not merely
   successful submissions. The parent assigns material repairs back to the
   builder and asks the verifier to recheck the affected claims.

Final acceptance includes required build/checks and Go tests, production/dev
browser behavior without skips, and direct visual inspection of affected flows.
Use existing suites; introduce no runner, evidence ledger, or alternate harness.
Deliberately restored mutations must demonstrate that the new ctx and naming
assertions detect the failures they claim to catch. Reuse still-applicable prior
mutation evidence; repeat expensive suites only when changed behavior or an
unresolved failure warrants it.

Historical operational launches and earlier naming decisions are retained in
Git history and the existing worklog. They are not current instructions. The
previous implementation's independent results are in
`ephemeral/tmp/remote-params-independent-qa-final.log`; they establish the
baseline, not acceptance of the revised public API.
