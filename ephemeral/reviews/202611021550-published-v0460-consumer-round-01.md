# Published v0.46.0 consumer proof — adversarial review, round 01

Outcome: **material findings remain**.

## Target and scope

Reviewed commit `b966ffe0375e3abe3cdeb40a5a7e9cf9ac552123`, specifically
`SVELTEKIT_COMPATIBILITY.md:141–204` and
`ephemeral/worklog/202611021538-published-v0460-consumer.md`, against the request
to verify published v0.46.0 GitHub Release, Go proxy, npm adapter/add-on and
Homebrew channels; fresh consumer creation/build/run with Kit 3.1.1; and a
published `skgo update` from v0.45.1 preserving authored edits. Only published
artifacts qualify. This is a proof review, not a general release qualification
or a request to modify the implementation.

The review was performed directly, without another agent. No scope-narrowing
instruction was supplied or ignored. Only this review file was written in the
repository. The pre-existing untracked `.playwright-mcp/` directory was read
and left intact. Consumer sources, snapshots, caches and binaries were not
changed; the updater and creation commands were not rerun.

## Evidence inspected and independent checks

Here `S` denotes the supplied evidence root:

```text
/private/tmp/claude-501/-Users-tyler--codex-worktrees-windows-native-shell-skgo/d15f6fa7-f266-4c79-82ca-f969d2a10287/scratchpad
```

Read the complete target diff, surrounding compatibility entry, repository
instructions, current-Kit guidance, and updater code in
`internal/update/{update,complete,frontend}.go`. Inspected the retained
creation/update logs, both applications, `app45-pre.tgz`, pre/post files and
hash manifests, release archives/executables, npm tarballs, scoped module
cache, and installed Homebrew receipt/cask/binary. Also inspected the two
original browser DOM snapshots in `.playwright-mcp/`.

| Requirement | Evidence and result |
| --- | --- |
| GitHub Release | Independently read the [v0.46.0 release API](https://api.github.com/repos/tylergannon/skgo/releases/tags/v0.46.0). It is published, stable, and has all three named platform archives. Publication time and archive digests match the journal. Rehashed the retained Darwin archive: `a450fab3c9bc197c76afe237e84cf34ecbb0a5696d592f3889894a6c3ac70506`. Its embedded executable, `S/cli/0.46.0/skgo`, the updated `S/cli/update-run/skgo`, and Homebrew's executable all hash to `4516e490fa2f10ea98488c21f04e4b0699eb19aaedfb1159aba195441a906c0b` and report v0.46.0/go1.27.3. The original retained v0.45.1 archive's embedded executable matches `S/cli/0.45.1/skgo`, which reports v0.45.1. Linux executables were not run. |
| Go proxy publication | Independently fetched the public proxy's [.info](https://proxy.golang.org/github.com/tylergannon/skgo/@v/v0.46.0.info), [.mod](https://proxy.golang.org/github.com/tylergannon/skgo/@v/v0.46.0.mod), and [.zip](https://proxy.golang.org/github.com/tylergannon/skgo/@v/v0.46.0.zip). Origin commit, tag and time match the journal. Recomputed the Go directory hash over the ZIP's 648 entries: `h1:fZLpHssyE/ZBUCNWBbgyYkFc1axZP7EHZmukc9sx9lU=`. Every file matches the retained scoped-cache ZIP. This establishes public proxy availability and content identity, separately from the update's transport; see finding 1. |
| npm adapter and sv add-on | Independently read the [adapter registry metadata](https://registry.npmjs.org/@skgo/sveltekit-adapter) and [add-on registry metadata](https://registry.npmjs.org/@skgo/sv). Both latest tags are 0.46.0, with the recorded publication times and integrity strings; the adapter's Kit peer is exactly 3.1.1. SHA-512 of each retained 0.46.0 tarball matches registry integrity. Every file in the retained adapter tarball matches both consumers' installed adapter. The two retained add-on versions' `sv-addon.js` hashes are equal (`d55e495f30a3292213ebb90bfe258cdf5212145c74027eb7aa31f8020fbfc7b8`). `new-fresh46.log` records actual published add-on resolution and successful creation. |
| Homebrew | Read the [cask at b9ec1c5a](https://github.com/tylergannon/homebrew-skgo/blob/b9ec1c5a/Casks/skgo.rb), installed metadata, symlink and executing binary. Version, release URL, archive checksum, and installed executable identity match. The tap commit timestamp is 2026-11-02T21:31:44Z. |
| Fresh consumer | `S/logs/new-fresh46.log` records published add-on/adapter installation, Kit correction, generation, frontend build and `exit=0`. Retained manifests require released SKGo v0.46.0 and adapter 0.46.0/Kit 3.1.1, without a local replacement. `go version -m S/consumers/fresh46/bin/fresh46` identifies the released v0.46.0 dependency and expected module sum. Independently ran that retained binary: `/` returned 200 HTML, `/_app/version.json` returned 200 JSON, and a missing route returned rendered 404 HTML. |
| Published edited-consumer update | `S/logs/new-app45-retry.log` records successful v0.45.1 creation. The pre-update archive matches every pre-snapshot file. `S/logs/update-app45.log:1–5` records v0.45.1 replacing itself with v0.46.0; lines 7–10 and 42 record staged correction replacement/verification; lines 62–85 record the Go version/module upgrade and actual project tool identity; lines 95–102 record the installed adapter/Kit upgrade and correction verification; lines 103–181 record generation, application build, completion and `exit=0`. The final executable embeds the released v0.46.0 dependency and sum. |
| Authored edits and correction | Verified both snapshot hash manifests against the snapshot files, and inspected pre/post diffs. README, Go middleware, home page, notes page, Go remote query and static asset are byte-identical. The script, nanoid dependency and override survive. The recorded manifest normalization, Vitest override and adapter release-age exception accurately describe the additional changes. The old correction file is gone, the new file has the expected patch digest, and both installed Kit queue files hash to `400bf34544e2b3c5512e489eb5a0ca99a7483f0b6d733f70cab9e81e28f3a0f5`. |
| Updated application behavior | Independently ran the retained `app45` binary. `/` and `/notes` returned 200 with `X-App45: authored-hook`, the authored heading, and both literal notes. `/edited.txt` returned 200 with the retained authored file's exact body. A fresh Chromium session navigated from home through the notes link: exactly one document request, one navigation entry, `/_app/remote/o0aeqp/getNotes` returning 200, both notes rendered, no console messages and no page errors. Reviewer-owned servers/browser were stopped. |

The independent runtime checks used the existing consumer binaries and no
checkout build. The original DOM snapshots support the visible page contents;
they alone do not record console/network activity. The fresh reviewer browser
check above supplies independent confirmation of that behavior. Separate
transcripts for the claimed original Vitest runs and original pre-update HTTP
probes were not present among the retained logs; those historical executions
were not independently re-established. Scaffold component tests are not used
as evidence of the Go application's client handoff.

The disclosed `skgo check` failures remain failures; this review does not
promote them to passes. Neither a clean `skgo check` nor advancing unrelated
native-shell examples was a condition of the consumer task as supplied.

## Findings

### 1. Issue — the claimed update through proxy.golang.org is not established; the retained cache records direct Git fetching

**Locations:** `SVELTEKIT_COMPATIBILITY.md:180–183` and
`ephemeral/worklog/202611021538-published-v0460-consumer.md:9`.

The journal says the update fetched v0.46.0 through proxy.golang.org. The
worklog treats the presence of `cache/download/.../@v/v0.46.0.{info,zip}` as
proof of that hop while acknowledging `GOPRIVATE=github.com/tylergannon/*`.
That inference is incorrect. Go writes the same cache files for direct VCS
downloads, and `GOPRIVATE` defaults both `GONOSUMDB` **and `GONOPROXY`**.
Matching modules bypass `GOPROXY`. These behaviors are explicitly documented
in the [Go module reference](https://go.dev/ref/mod#private-modules) and its
[proxy/cache description](https://go.dev/ref/mod#goproxy-protocol).

Concrete retained and live evidence:

```text
go env GONOPROXY GOPROXY GOPRIVATE GONOSUMDB
github.com/tylergannon/*
https://proxy.golang.org,direct
github.com/tylergannon/*
github.com/tylergannon/*
```

`S/gomodcache/cache/vcs/958b3440903d2a08b631a5a01f479851ae82c642bebf7a0d160c56a3594ba30f/config:7–9`
identifies `https://github.com/tylergannon/skgo` as the direct Git origin.
That directory's `FETCH_HEAD:1` records:

```text
e5665b8a3c8c84b974042f317b281d8989dbe0b2    tag 'v0.46.0' of https://github.com/tylergannon/skgo
```

The fetch record's modification time is 2026-11-02 15:43:09 local, during the
update whose log ends at 15:43:39. Its shallow commit set includes the
expected `45e471a8...` release commit. The updater inherits the process
environment (`internal/update/update.go:50–54`); the recorded update log does
not capture an override disabling `GONOPROXY` or a proxy request trace.
Consequently the cache cannot establish the claimed transport, and its VCS
records support direct fetching during the update.

**Impact:** the published update and released module identity are supported,
but the target overstates which consumer channel that run exercised. A
working direct Git fetch could mask a broken public Go proxy path. The
independent live proxy ZIP verification in this review establishes that the
channel currently serves the right bytes; it does not make the original
update's proxy-hop assertion true.

**Required correction:** describe the actual direct-fetch evidence and
separate it from independent public proxy availability. If the intended claim
is an end-to-end update through that proxy, repeat from a preserved pre-update
copy with an empty scoped module cache, `GOWORK=off`, `GONOPROXY=none` and
`GOPROXY=https://proxy.golang.org` (no direct fallback), capturing the effective
settings and result. Do not infer transport from cache layout or matching
module hashes. No repair or update rerun was performed in this read-only review.

### 2. Nitpick — the concurrent add-on download collision is asserted as a confirmed cause

**Locations:**
`ephemeral/worklog/202611021538-published-v0460-consumer.md:6` and
`SVELTEKIT_COMPATIBILITY.md:200–202`.

`S/logs/new-app45.log:54–67` establishes a failed community-add-on download
with `Unexpected end of input`; `new-app45-retry.log:48–63` establishes that
the serial retry succeeded. The valid retained tarball excludes a persistently
malformed published package. These observations do not isolate a shared-cache
collision from a transient download/import failure. No controlled concurrent
reproduction or cache-write trace was retained.

**Impact and correction:** this does not invalidate either successful consumer
run. Describe concurrency as the observed circumstance and a collision as a
hypothesis, rather than the established diagnosis. Further debugging is not a
prerequisite to the requested proof review.

## Disposition

**Material findings remain:** one issue in the claimed Go-proxy transport and
one causal wording nitpick. The release identities, public channel availability,
fresh consumer runtime, released updater completion, preserved authored edits,
installed Kit correction, and updated application's HTTP/client behavior are
otherwise supported by the inspected artifacts and independent checks above.
