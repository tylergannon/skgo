# Rejected development-loop optimizations

Ideas for making `skgo dev` faster that were considered and rejected. Each
entry records why, what was done instead, and what would reopen it. Timings
come from the skgo-project ledger (`ledger/readings.tsv`) or from timers
placed in the code temporarily.

## Race a hot patch against a complete rebuild

*Considered 2026-10-19, during skgo#411.*

The idea was to start the hot patch and the complete rebuild together on every
Go edit and keep whichever finishes first.

**Rejected** because the hot patch classifies the edit before it loads any
types, so a declaration change already reaches the rebuild within
milliseconds. A compatible body edit patches in about 60 ms. A rebuild never
has a viable competitor, so running both would only take CPU from the build.

**Reopen** if classification becomes slow, or if it has to guess and is
sometimes wrong.

## Patch polytype's cached discovery between codegen phases

*Considered 2026-10-19, during skgo#413.*

Codegen loads packages in three phases. Each phase adds generated files, and
polytype keys its discovery on those files' headers. So an edit that changes
imports runs one `go list` per phase, about 490 ms in total. The idea was to
have polytype build the later phases' metadata from the first phase's result,
whenever the added files sit in packages it already knows and import only
packages it already knows.

**Rejected** because polytype would have to predict `go list`'s answers itself,
including build constraints, cgo and embed rules. That is partly
reimplementing Go's package selection, and a wrong prediction means
type-checking against stale metadata. The payoff is about 300 ms, and only on
import-changing edits. The common edit (a struct field plus the component that
uses it) already spends 3–6 ms per codegen load.

**Reopen** if import-changing edits are shown to be frequent and painful, and
only after `go list`'s cost per call has been cut first.

## Show early codegen phases the previous generation's files

*Considered 2026-10-19, during skgo#413.*

The idea was to present every phase with the previous pass's generated files,
so that all three phases share one discovery key.

**Rejected** because the first phase validates the locals package before
`LocalsFrom` is regenerated. Stale generated code could make it type-check
against declarations the edit removed.

## Ask for export files on every retained metadata request

*Considered 2026-10-19, during skgo#413.*

The hot patch's metadata request and its typed request were two discoveries.
The idea was to widen every metadata request to the typed request's mode, so
the two would share one discovery.

**Rejected** because once any captured source differs, polytype rediscovers
every request that asks for export files, since it needs fresh compiler
artifacts. Every patch after the first would then run `go list`.

**Done instead:** only the baseline request, made before any source changes,
asks for export files.

## Preload each package's baseline with its own analysis

*Considered 2026-10-19, during skgo#413.*

**Rejected** because every hot-patchable package would run its own `go list`
after every build, about 200 ms each.

**Done instead:** a host's packages share one analysis, and one background
preload fills it.

## Pre-pay macOS's first-exec check for a new host

*Considered 2026-10-20, during skgo#418.*

macOS checks a newly written executable the first time it runs, which takes
about 230–300 ms per file. Copying the same bytes to a new file pays it again.
The idea was to pay it during the build's idle tail: launch the new binary
under ptrace, so it stops before running any of its code, then kill it with
`PT_KILL`. (A plain `SIGKILL` leaves the traced child stuck.) This works: after
the probe, starting the binary takes about 50 ms.

**Rejected** because nothing is left to overlap. Once the rebuild stopped
waiting between the build and `Start`, only about 18 ms separates them, and the
probe takes the same ~240 ms on the critical path.

**Untested alternative:** listing the terminal that runs `skgo dev` under
System Settings → Privacy & Security → Developer Tools is reported to exempt
its child processes from the check.

**Reopen** if work is found that can run between the build and `Start`.

## Reuse formatted output for unchanged generated frontend files

*Considered 2026-10-20, during skgo#418.*

Codegen formats its generated frontend files with a `vp fmt` subprocess on
every generation, about 90 ms. The idea was to remember each file's formatted
text by its generated text, and skip the subprocess when nothing changed.

**Rejected** because neither edit it targets reaches it. An API field edit
changes `+page.server.ts`, so one file still needs formatting and the cost
stays. A body edit reuses codegen's cached plan and never formats anything.

**Reopen** if a common edit is shown to regenerate without changing any
frontend file.

## Give compiler inputs a stable directory

*Considered 2026-10-20, during skgo#418.*

Each revision is compiled from a fresh private tree, and the idea was that a
fresh directory defeats Go's build cache.

**Rejected** because the host builds with `-trimpath`, so the directory is not
part of the cache key. A `go build` after an API field edit compiles only the
four packages the edit reaches (the route, the bindings, the application and
`main`), then links.

## Keep the host's `go list -export` from compiling downstream packages

*Considered 2026-10-20, after skgo#419.*

On an API field edit, the host load (about 270 ms) ends with about 115 ms of
serial compiles. `go list -export` compiles every package it prints, so it
compiles the edited route link, the bindings, the application and the
production `main`. The build then compiles the first three again with its
overlays. `go/packages` type-checks those roots from source, so their archives
only end up in the hot-patch importcfg. A route never imports the packages
that depend on it.

**Rejected** because skipping those compiles takes a second discovery. Go
offers no way to export only some of the printed packages. To print just the
dependencies, the load first needs the roots' imports from a metadata pass.
On the example with a warm cache, a metadata-only `go list -deps ./...` costs
the same ~270 ms as the export pass, which is twice what the compiles cost.
Polytype's `View` does not help: its metadata pass asks for export files too,
and each revision's compiler tree has a new path, so retained discovery misses.

**Reopen** if package metadata can be carried across revisions without a
`go list`, for example from a compiler tree with a stable path.
