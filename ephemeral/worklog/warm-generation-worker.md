correction: Current SKGO readMarker accepts a named *types.Func and rejects function-literal marker arguments; test the released authoring form rather than importing an older API assumption.
decision: Projection-plan reuse excludes only parsed named-function bodies. Const and variable initializers, declaration comments/tags, imports/build constraints, membership, controls and embed inputs remain exact. This is not compiler validation: the dev owner must build fresh executable source after every successful Worker.Run.
correction: Polytype-generated headers identify a backend, not the owner. SKGO now labels its emitted type projections with its own header and prunes generic legacy headers only in its reserved src/lib/skgo directory. Ambiguous legacy colocated Polytype files are retained until a producer establishes ownership; deleting arbitrary foreign output would repeat #313.
friction: WIP4939b13 predates the released narrow parser-source overlays. Porting it wholesale would restore broad overlays and debug output; retained private materialization was ported separately while preserving the released loader behavior.
friction: A controlled real-example field edit was initially measured while the local Polytype replacement was still being edited; conservative source-change verification reran generation. Freeze all mutable producer modules, not just the host repository, or benchmark a frozen copy. The corrected run omitted 20 of 21 TypeScript backend calls and matched all 186 generated outputs.
decision: Warm CPU sampling attributed 350ms cumulative to projection fingerprinting and 400ms to capture across eight body edits (1.27s sampled CPU, 2.74s wall). Exact-content per-file projection digests remove unchanged-file parsing; no timestamp/TTL validity shortcut was added. Directory/file acquisition remains separately measurable work.
friction: The example's placeholder embedded build lacks skgo.manifest.json, required even with --proxy. Generation/real Go build and literal compiled-handler checks can run independently; they are not an HTTP/browser startup proof. The supervisor lane supplies the complete frontend-built application.

- Real API profiling found embedded-directory sentinels were passed with trailing
  separators to `goload.View`; its snapshot clone cleaned keys but its header
  index did not. Each declaration edit consequently looked structural and
  discarded metadata. Normalize sentinels at capture; Polytype also repaired its
  public boundary. The embed-directory + field-edit regression requires no new
  metadata acquisition. Frozen example attribution dropped from 27 metadata Go
  subprocesses (3.395s aggregate) to zero. The remaining 3 env/2 package-dir calls
  total about85ms; only one of21 TS producers runs. Instrumented wall time was
  5.867s before and1.698s after, with concurrent machine activity: use the causal
  call elimination, not these as isolated latency promises.
- Exact-content projection memo proves one parse per edited source file, but
  frozen body CPU profiling now points chiefly to filesystem capture (950ms of
  1430ms sampled CPU over8 edits). Do not claim the memo eliminates source
  acquisition, or weaken exact capture using timestamps/TTL to chase that cost.
- Warming discovery changed root-package return order and therefore generated
  `skgoN` import aliases; the real example's exact-output comparison caught two
  differing bindings even though replacing each alias with its import path made
  both whole files identical. Sort the returned root set by PkgPath before
  allocating aliases. Reversing discovery output is the focused regression;
  removing the sort must fail it. This avoids output churn without treating
  semantic equivalence as an excuse to relax exact authoritative comparison.
