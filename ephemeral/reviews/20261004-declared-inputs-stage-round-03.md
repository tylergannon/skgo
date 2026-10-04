# Declared prerender Inputs stage review — round 03

Outcome: **material findings remain**.

Review target: frozen Go/API/generator checkpoint `598c1389012306bba78a911693002f5c92674cdb`, with design corrections through `198778e77f736b62ac27cb8aa094ac73729cccf7`. This is an early implementation-stage review, not full Inputs qualification or merge approval. The adapter is explicitly still pending; its absence is not counted as a defect in a completed feature. The full authorized goal and acceptance groups remain authoritative when judging implemented boundaries.

## Evidence inspected

Applied the repository agent-protocol/adversarial-review instructions already loaded in this session. Read the entire checkpoint diff, surrounding scanner, name allocator, codec planner, emitter, runtime transport and Inputs dispatcher, and new tests. Revisited the complete worklog and earlier immutable design reviews. Read the independent Go-owned native reference at `../prerender-inputs-proof/native/native_test.go`, `native-final.log`, bootstrap-removal log, actual initialized/stock/removal fixture sources, calls receipts, failure stacks and produced Money artifact. The installed Kit 3.0.0 source mapped in prior rounds remains the native authority.

No product/test/dependency files were written and no additional test/build run was performed by this reviewer. Findings below give concrete source-derived reproductions, not a claim of reviewer-executed failures. Only this new immutable review artifact was written. No caller-suggested defect exclusion or desired verdict constrained the review.

## 1. Issue: generated transported Inputs are flattened before transport reducers see them

**Requirement:** worklog acceptance group 4 requires Money(125) to preserve transport, its native key and Go body value. The reviewed encoder boundary requires transported input types to use Call.Transported.

**Evidence:** `internal/gen/codecs.go:90–94` assigns an input codec to every argument-taking prerender, including Money. At the frozen checkpoint, `internal/gen/emit.go:694–708` chooses Call.Transported only when `fn.inCodec == ""`; the normal path calls `Encode<inCodec>(value)`. Thus the transported path is unreachable for these prerender producers. The generated codec turns Money into its ordinary devalue object tree. `transport.go:128–144` recognizes transported values by their original Go reflect.Type, which that object no longer has.

**Reproduction:** register transported Money, declare `func amount(context.Context, Money) (string,error)` and `func amounts() ([]Money,error)` returning Money(125), generate, then invoke its remote-inputs command. The generated Inputs closure calls EncodeMoney/its root alias rather than Call.Transported. The response has an ordinary object instead of a Money transport tag; native initialization in the adapter cannot restore the lost type. This prevents the required Money class/key behavior before any worker-boundary question arises.

**Correction/proof:** decide producer encoding using whether the input type reaches a transported type, independently of the strict decoder codec's existence. Retain the input decoder for body calls. Exercise the actual generated producer command for both Money and a container reaching Money; assert the literal transport tag/value and then the native-key integration. A hand-authored RemoteSpec closure cannot catch this generator bug.

## 2. Issue: a noarg-only Inputs app receives an unused fmt import

**Requirement:** group 3 explicitly supports an optional `func() ([]devalue.UndefinedValue,error)` producer on a no-argument remote, including an empty result; generated Go must compile.

**Evidence:** `internal/gen/emit.go:582–583` imports fmt whenever any Inputs producer exists. `hasPrerenderInputs` at lines 675–681 includes noarg declarations. The noarg emitter branch at lines 698–701 returns without emitting any fmt use. Normal noarg remote handlers also do not use fmt. `writeGo` uses go/format, which does not remove unused imports.

**Reproduction:** an otherwise minimal app has only `func empty(context.Context) (string,error)` and `func inputs() ([]devalue.UndefinedValue,error)`, registered with Inputs. Generate and build its generated package: Go rejects the unused fmt import. The checkpoint's combined string-plus-noarg fixture conceals the failure because its string producer emits fmt.Errorf.

**Correction/proof:** import fmt only when emitted code needs it. Add a noarg-only generated-app compile fixture; it must not contain an unrelated argument-taking producer or batch handler that supplies a fmt use.

## 3. Issue: input closure names collide across valid remote modules

**Requirement:** additive Inputs must preserve existing module-scoped remote naming and generated-app compatibility. Two separate remote modules may publish the same export name.

**Evidence:** `internal/gen/emit.go:684` names every Inputs closure solely `"inputs_" + fn.name`; lines 691–697 emit that package-level function and registration uses the same spelling. In contrast, existing `internal/gen/names.go:18–34` allocates unique remote handler names across modules, and `scan.go:805–814` rejects only duplicate module/name pairs.

**Reproduction:** two valid local publication packages each declare a remote named `item`, each with its own typed Inputs producer. Generation writes two `func inputs_item` definitions into the single app bindings package, and both registrations use that identifier. The app cannot compile, though the equivalent two remotes without Inputs are supported.

**Correction/proof:** allocate Inputs closure names through the existing unique-name mechanism or a collision-free derivation from its allocated remote identity. A two-package same-export fixture must compile and invoke each generated producer independently, asserting distinct literal arrays so misregistration cannot pass.

## Native transport bootstrap adjudication

The retained reference evidence substantiates the narrow bootstrap decision. `native-final.log` records the stock transported-inputs failure, initialized native success, bootstrap-only removal failure, and the separate canonical-versus-IPC assertion. The actual stock/removal stacks fail at untouched Kit `core/postbuild/prerender.js:721` with `Cannot stringify arbitrary non-POJOs`. Comparing initialized and removal hooks shows only the diagnostic init_transport tail removed; their producer mode and handled-error policy remain the same.

The initialized fixture passes its emitted hooks' own transport object to the physical installed native module. Its body checks `instanceof Money`, and the calls receipt contains exactly one `body:amount:125`. The retained artifact at `.../initialized-inputs/.svelte-kit/output/prerendered/data/_app/remote/3215r6/amount/W1siTW9uZXkiLDFdLFsyXSwxMjVd` contains the literal money:125 result. The reference test asserts exact registered paths, bodies and call receipts. Removing initialization requires both build failure at the native boundary and absence of that Money artifact.

The native source-only object-payload control independently asserts canonical key `W1siTW9uZXkiLDFdLFsiX19za3JhbyIsMl0seyJjZW50cyI6M30sMTI1XQ` versus IPC `[["Money",1],{"cents":2},125]`; it avoids treating the array-payload reference's key as universal. These receipts support worker-local native initialization as an explicit workaround for the stock Kit 3.0.0 defect. They do not prove the pending skgo helper performs it correctly, nor can they compensate for finding 1's earlier Go type erasure.

The concrete producer type requirement is now correctly reflected in the checkpoint scanner; the prior []any-only draft is gone. The added dispatcher invokes Inputs with background context and no Event, propagates producer errors, and rejects unknown request fields. No further material finding is reported in that reviewed stage. Existing focused tests and native reference runs are insufficient to qualify all seven groups; the three generator corrections and later real adapter/lifecycle/runtime proofs remain outstanding.
