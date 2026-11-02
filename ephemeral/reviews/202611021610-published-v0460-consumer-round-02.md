# Published v0.46.0 consumer proof — adversarial review, round 02

Outcome: **no findings**.

## Target and authority

Reviewed the entire current published-consumer proof at commit
`cbaeed75acc488d68f533f4c884136d295c4fa48`:
`SVELTEKIT_COMPATIBILITY.md:141–218` and
`ephemeral/worklog/202611021538-published-v0460-consumer.md`. Read the full
current text, its surrounding compatibility entry, the changes since the
published-main base `45e471a8`, and the prior review. This was not limited to
checking the previous findings.

The authoritative task remains: verify published SKGo v0.46.0 GitHub Release,
Go proxy module, npm adapter and sv add-on, and Homebrew cask; fresh consumer
creation/build/run with released Kit 3.1.1; and published `skgo update` of an
edited v0.45.1 consumer, using published artifacts only. Repository instructions
and the current-Kit guidance were reread. No instruction narrowed the permitted
findings or requested a verdict.

The review was performed directly, without delegation. Only this review file
was written in the repository. The existing `.playwright-mcp/` material and all
consumer sources, binaries, snapshots and caches were left unchanged. No
creation, dependency installation, update, or rebuild was performed by this
reviewer.

## Evidence inspected

`S` below denotes:

```text
/private/tmp/claude-501/-Users-tyler--codex-worktrees-windows-native-shell-skgo/d15f6fa7-f266-4c79-82ca-f969d2a10287/scratchpad
```

Inspected the original creation/update logs and consumers, both published CLI
archives, the npm tarballs, `app45-pre.tgz`, original snapshots and hash lists,
the original scoped module cache, and Homebrew's installed receipt and binary.
Inspected the new `rerun/app45`, `cli/update-run3`, `gomodcache3`,
`snap/rerun-pre.sha256`, `snap/rerun-post-authored.sha256`, and the full
`logs/update-app45-proxy.log`. Read the relevant updater environment inheritance,
release handoff, dependency alignment and Go update code in
`internal/update/{update,complete,frontend}.go`.

Public sources were checked again during this round:

- [GitHub Release API](https://api.github.com/repos/tylergannon/skgo/releases/tags/v0.46.0).
- [Adapter registry](https://registry.npmjs.org/@skgo/sveltekit-adapter) and
  [sv add-on registry](https://registry.npmjs.org/@skgo/sv).
- Go proxy [.info](https://proxy.golang.org/github.com/tylergannon/skgo/@v/v0.46.0.info)
  and [.zip](https://proxy.golang.org/github.com/tylergannon/skgo/@v/v0.46.0.zip),
  and the public [checksum lookup](https://sum.golang.org/lookup/github.com/tylergannon/skgo@v0.46.0).
- [Homebrew cask at b9ec1c5a](https://github.com/tylergannon/homebrew-skgo/blob/b9ec1c5a/Casks/skgo.rb).
- [Go module reference](https://go.dev/ref/mod#private-modules) for the
  proxy/private-module distinction.

## Assessment against the whole task

| Claim | Verification and result |
| --- | --- |
| GitHub Release and executing CLIs | The release is published and stable at the recorded time, with all three named platform archives and matching API digests. Rehashed the retained Darwin archive: `a450fab3c9bc197c76afe237e84cf34ecbb0a5696d592f3889894a6c3ac70506`. Its embedded executable matches `cli/0.46.0/skgo`; both update-run executables and the installed Homebrew executable match the same binary digest, `4516e490fa2f10ea98488c21f04e4b0699eb19aaedfb1159aba195441a906c0b`. Each retained CLI's `buildinfo --json` reports the appropriate released module/version, including the original v0.45.1 CLI. The original v0.45.1 executable also matches its archive. |
| Published Go module | Live proxy metadata identifies tag v0.46.0 at `45e471a8e6e2677ffb8a6a49f4a680a78e7d4d21`, with the recorded timestamp. Recomputed the Go directory hash from the live ZIP: `h1:fZLpHssyE/ZBUCNWBbgyYkFc1axZP7EHZmukc9sx9lU=`. All 648 ZIP entries match `gomodcache3`'s retained ZIP and extracted module files. The public checksum lookup matches the retained lookup and expected module/go.mod sums. |
| npm adapter and add-on | Both live latest tags remain 0.46.0; publication times and SHA-512 integrities match the journal and retained tarballs. The adapter peer is exactly Kit 3.1.1. Every retained adapter tarball file matches the installed adapter in `consumers/fresh46`, `consumers/app45`, and `rerun/app45`. The old/new add-on `sv-addon.js` files have the same digest, as recorded. `new-fresh46.log:48–59` records use of the published 0.46.0 sv add-on; successful generation follows. |
| Homebrew | The published cask version, release URL and SHA-256 match. The installed receipt names tap head `b9ec1c5ac00cfba2ae8a524f3374d56d29e049f2` and version 0.46.0. `/opt/homebrew/bin/skgo` resolves to that cask's executable and matches the released executable digest. The documented initial-install substitution for a previously untapped cask is supported; no reinstall was needed for this read-only review. |
| Fresh creation/build/run | `new-fresh46.log` records released add-on/adapter installation, Kit 3.1.1 correction, generation, frontend build and `exit=0`. The retained app requires released Go module v0.46.0 and adapter 0.46.0/Kit 3.1.1. Its standalone executable embeds the expected released module and sum, without a replacement. Independently ran it: `/` returned 200 HTML, `/_app/version.json` returned 200 JSON, and a missing route returned rendered 404 HTML. |
| Original edited v0.45.1 update | `new-app45-retry.log` records successful released v0.45.1 creation and the 3.1.0 correction. The archived pre-update application contains the authored edits and v0.45.1 requirement. The original update log records replacement of the executing CLI, dependency/correction upgrades, generation, application build and successful completion. The journal now correctly describes its module transport as direct Git. Its current binary independently served the authored home, Go-rendered notes and static asset; home and notes responses carried the authored middleware header. |
| Proxy-only published update rerun | `update-app45-proxy.log:1–8` captures effective `GOPROXY=https://proxy.golang.org`, `GONOPROXY=none`, `GOPRIVATE=none`, `GONOSUMDB=none`, `GOSUMDB=sum.golang.org`, `GOWORK=off`, and scoped `GOMODCACHE=.../gomodcache3`. Lines 10–14 record the actual v0.45.1 CLI downloading/replacing itself with v0.46.0 and selecting adapter/add-on 0.46.0. Lines 16–19 and 51 record staged correction replacement/verification; lines 52–94 record downloads, Go/module upgrades and the actual project tool verification. Lines 104–111 record the installed adapter/Kit upgrade and correction verification. Lines 112–190 record generation, frontend and standalone application build, completion and `exit=0`. The scoped cache has no `cache/vcs`, contains the expected proxy module artifacts and checksum lookup, and its files match the public proxy. These settings and results support the claimed proxy hop rather than relying on cache filenames alone. |
| Authored edits preserved in both runs | All 12 entries in `rerun-pre.sha256` match the archived pre-update files. Both original hash manifests verify. The six authored files—README, Go middleware, home page, notes page, remote Go query and static asset—are byte-identical across the archive, original pre/post snapshots, original updated consumer and proxy rerun. The rerun post-authored hash list verifies against the actual rerun files. Authored script, nanoid dependency and override survive. Final go.mod, go.sum, package.json and workspace files are byte-identical between the two updated consumers; the documented normalization and managed additions accurately describe the changes. |
| Released Kit pairing and correction | All three installed consumers use adapter 0.46.0 and Kit 3.1.1, with no local module replacement or file/link package reference in the inspected dependency configuration. Only the new owned correction file remains. Their installed queue source hashes to `400bf34544e2b3c5512e489eb5a0ca99a7483f0b6d733f70cab9e81e28f3a0f5`. Each retained application executable identifies released SKGo v0.46.0 with the expected module sum and Go 1.27.3. |

### Independent runtime check of the proxy rerun

Started `S/rerun/app45/bin/app45` directly on a reviewer-owned dynamic loopback
port, without rebuilding or using a checkout executable. HTTP checks returned:

- `/`: 200, authored home heading, `X-App45: authored-hook`.
- `/notes`: 200, both literal authored notes, `X-App45: authored-hook`.
- `/edited.txt`: 200, exact literal body `authored static asset in app45\n`.

In a fresh Chromium session, observed the home heading before hydration and
confirmed the same DOM node remained after Kit set manual scroll restoration.
Clicked the notes link and observed one document request total, one navigation
entry, `/_app/remote/o0aeqp/getNotes` returning 200, and both `authored before
update` and `served by Go` rendered. There were zero console messages and zero
page errors. This confirms the new rerun's browser behavior, independently of
the earlier consumer's browser evidence. All reviewer-owned servers and the
browser were stopped.

### Supporting tests and limits

Located the existing Vitest result caches under both original consumers at
`web/node_modules/.vite/vitest/da39a3ee5e6b4b0d3255bfef95601890afd80709/results.json`.
They record `failed: false` for `greet.spec.ts` in the server project and
`Welcome.svelte.spec.ts` in `client (chromium)`, with timestamps during the
original verification. Each source file contains one test. These support the
reported two-test passes; they are cached results rather than complete runner
transcripts and do not prove the Go application's client handoff. The fresh
browser check above supplies that application-level evidence for the rerun.

No new Vitest run or general repository suite was necessary for this proof-only,
read-only review. The original pre-update HTTP execution was not repeated;
its preserved source/archive, creation evidence and server log remain the
historical evidence. Linux executables were not run. The task does not claim
exhaustive platform or Kit compatibility from these consumer probes.

The disclosed `skgo check` formatting/staticcheck failures remain limitations,
not passes. The current proof does not claim a clean `skgo check`. The pending
`docs/` version text and retained native-shell example pairing are disclosed
and are not omitted consumer-channel requirements in the supplied task.

## Prior findings and current disposition

1. **Round-01 issue resolved.** `SVELTEKIT_COMPATIBILITY.md:184–196` and worklog
   line 9 correct the original direct-fetch attribution and distinguish the new
   proxy-only update. The effective settings, isolated cache, checksum record,
   update log, released executable identities and working rebuilt app support
   the replacement claim.
2. **Round-01 nitpick resolved.** `SVELTEKIT_COMPATIBILITY.md:212–216` and worklog
   line 6 now distinguish the observed overlap/failed download from a proposed
   shared-download collision. They no longer assert an established cause.

**No findings.** No material defect or genuine remaining nitpick was found in
the current published-consumer proof against the authoritative task. This
conclusion applies to the named consumer verification and its stated limits.
