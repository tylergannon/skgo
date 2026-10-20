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
