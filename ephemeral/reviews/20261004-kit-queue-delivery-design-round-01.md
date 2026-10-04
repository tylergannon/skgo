# Kit queue correction delivery design review — round 01

Outcome: **no findings**.

Target: `ephemeral/worklog/20261004-kit-queue-delivery-design.md`, SHA-256 `ccf50c7cd9b71ad51d7f329df0729b88df7cbdb7556de65c88269abfba2a58f6`, including its prerelease/public-delivery distinction at line 65. Current product HEAD when inspected was `eb77292f397c77c305b0d64d0e750afba68d2947`. This is a design review, not implementation qualification, merge approval, or confirmation of a published fix.

The parent explicitly corrected the earlier agent-authored no-native-patch assumption and authorized the bounded queue correction and its consumer delivery. That revised authority supersedes the constraint recorded in Inputs review rounds 08 and 10. The authoritative scope still includes all seven Inputs acceptance groups, producer/body overlap, real process ownership and cleanup, generated JS/TS, and Go HTTP/Goja SSR. Neither worker assignments nor the requested review focus excluded other material defects. No delegation was used.

## Evidence inspected

Read repository instructions and the agent-protocol, adversarial-review and sveltekit-current skills; the complete delivery design and Inputs worklog; installed Kit `package.json`, `src/core/postbuild/queue.js`, its native prerender queue/seeding/body/error paths and call sites; current build helper resolution, transport and fatal-error paths; `internal/newapp` creation/finish/package selection and runner; Go and JS adapter fingerprint implementations; adapter npm file list and package tests; scaffold recipes, minimal Inputs regressions and native add-on integration documentation.

Independently verified the installed Kit version is 3.0.0. Reconstructed the proposed three-line semantic correction and unified diff in memory, without editing installed source. All three calculated SHA-256 values exactly match the design:

- Stock queue: `dbe8bbacab35cbd119d6f5bcf9b527ce0f7a1bfa63954b5ebe047df49f46c6d5`.
- Corrected queue: `400bf34544e2b3c5512e489eb5a0ca99a7483f0b6d733f70cab9e81e28f3a0f5`.
- Three-hunk patch: `0d35d370d3e5fb6e0c801cd1079013b3d487d6e301e27777f26fabbf6e0106c4`.

Read the installed native pnpm 12.9.1 package identity and its `config`/`config set` help. Executed only read-only project-scoped JSON gets: absent `patchedDependencies` returned JSON `null`, and the existing `overrides` map matched the selected example workspace. Native help explicitly describes `--location=project` as selecting the project configuration for reads and writes. The official [config documentation](https://pnpm.io/cli/config) confirms JSON object values and project YAML writes; [patch documentation](https://pnpm.io/cli/patch#patcheddependencies) confirms exact-version/range/name selectors, relative patch paths and selector precedence. These sources support the proposed mechanism, not a claim that its future merging or clean installation has already run. No native Rust command implementation was available locally; no claim about its internal serialization is made.

## Design assessment

The queue change addresses the reproduced lifecycle defect directly. Idle work may finish before `done()` without closing initial additions. Once seeding is declared complete, empty and active completion retain their existing behavior. Per-task execution, results, rejection, concurrency, and discoveries made by active tasks are untouched. Native prerender continues to interleave route work and awaited producers in its original order. There is no eager producer phase, synthetic task, altered key, or replacement crawler. Required stock-negative/corrected-positive and concurrency-1 overlap controls remain necessary because matching bytes alone cannot establish the behavior.

The proposed patch and metadata subtree participates naturally in both recursive fingerprints: Go embeds and walks `skgo-adapter`, and JS hashes the same paths and bytes. Explicit npm file-list additions plus the existing packed-versus-embedded byte tests address the actual public artifact boundary. A shared metadata accessor avoids independent Go/JS hash constants. The build guard checks the app's resolved physical package and queue bytes before Inputs IPC, and reports failure through the existing owner-drain path; a native ignore-500 policy cannot be treated as permission to continue an incompatible Inputs build.

Consumer configuration is bounded to selected project files and native package-manager installation. Explicit consent to a containing workspace, preflight conflict rejection, preservation of unrelated configuration, exact Kit pinning, and refusal to compose another Kit patch prevent accidental broad application of the correction. Separating apply from installed-byte verification avoids falsely reporting success after merely writing configuration. Leaving package-manager installation and lockfile mutation to native VP/pnpm preserves its ordinary patch integrity mechanism.

Both existing `newapp` branches converge on `finish`, so one shared Configure-before-install/Verify-after-install integration reaches fresh scaffolds without a second native add-on emitter. Existing consumers have the same Go-owned operation. The planned independent packed artifacts, immutable SDK fixture export, absence of checkout links, untouched decoy installation, and frozen reinstalls with independent stores target the delivery mistakes that a locally patched example would miss. Line 65 correctly separates prerelease package-shape proof from later actual Go-tag/npm-registry qualification.

## Remaining execution boundary

No material design defect was identified. This review performed no configuration set, install, package publication, native-source mutation or product test. Real native get/set merging and refusal controls, packed fresh/existing installs, corrected-native behavior, unchanged minimal artifact assertions, all seven Inputs groups, exact-head checks and ordinary application qualification remain required before readiness. The currently failing stock integration is not declared fixed by approving this design, and PR244 remains draft. No upstream communication or external PR is authorized by this review.
