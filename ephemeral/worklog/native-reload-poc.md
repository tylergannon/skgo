# Native reload POC

decision: Native reload stays in a separate experimental host: upstream darwin/arm64 uses cgo and private runtime internals; production SKGO remains pure Go/goja.
friction: A completed handler call does not alone permit unloading: a returned string may point into module data. Copy response bytes while holding the revision before releasing it.
decision: First bounded lifetime experiment freezes imports and function declarations, disallows custom types/globals/init and escaping execution, and drains all calls before unloading. Arbitrary app package replacement is outside the demonstrated boundary.
friction: Returning from a loader method invalidates the spike's stack-backed function-address cell; retain a heap address cell in each revision.
friction: Upstream RegSymbol consumes a fixed global function-pointer slot array and is not reentrant; use one registration per process.
friction: Go 1.27 test binaries strip runtime symbols by default; -ldflags=-s=false is necessary for this experimental loader.
correction: Darwin mincore does not report unmapped pages using Linux ENOMEM semantics; use Mach VM region lookup for an independent unload assertion.
correction: Native 19-24ms fixture timings exclude conservative dependency fencing; full fenced watched-file runs measured 254-451ms. Never present the former as complete dev-loop latency.
decision: The POC has no revision cache/TTL. Changed dependency bytes/membership or build environment require restart; source reverts compile fresh. Build-bound immutable snapshot integration is explicitly unimplemented.
