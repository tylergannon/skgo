decision: persist SKGO's formatted generated file plan at Run's analysis/publication boundary. Prepare must still return its promised live package graph; Check remains fresh. Persisting go/types or grammar results alone would miss substantial discovery, locals and hook costs.
decision: gopls caches serializable derived facts, hashes source/dependency inputs, namespaces storage by executable identity, and verifies stored payload content. Apply these principles to SKGO's own result; no shared module or alternative Go type system. Primary sources: https://github.com/golang/tools/blob/master/gopls/internal/cache/check.go and https://github.com/golang/tools/blob/master/gopls/internal/filecache/filecache.go.

trap: x/mod v0.42 WorkFile.SetUse retains an unchanged directive then appends it again. Before replacing workspace paths, DropUse existing entries and Cleanup; relative-only fixtures miss this failure.

trap: temporary module roots must be canonicalized before handing their paths to Go. On macOS os.MkdirTemp may return /var while Go resolves /private/var; mixing the two broke copied-example type projections and diagnostic rebasing.

trap: fresh private module paths change Go compiler action identities even with identical source. Private analysis prepends -trimpath to its captured GOFLAGS; caller flags remain last so explicit overrides survive. A compiler-export test compares two fresh roots and a literal tagged constant, rather than inferring reuse from elapsed time.
