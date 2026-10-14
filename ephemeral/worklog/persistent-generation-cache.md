decision: persist SKGO's formatted generated file plan at Run's analysis/publication boundary. Prepare must still return its promised live package graph; Check remains fresh. Persisting go/types or grammar results alone would miss substantial discovery, locals and hook costs.
decision: gopls caches serializable derived facts, hashes source/dependency inputs, namespaces storage by executable identity, and verifies stored payload content. Apply these principles to SKGO's own result; no shared module or alternative Go type system. Primary sources: https://github.com/golang/tools/blob/master/gopls/internal/cache/check.go and https://github.com/golang/tools/blob/master/gopls/internal/filecache/filecache.go.

trap: x/mod v0.42 WorkFile.SetUse retains an unchanged directive then appends it again. Before replacing workspace paths, DropUse existing entries and Cleanup; relative-only fixtures miss this failure.

trap: temporary module roots must be canonicalized before handing their paths to Go. On macOS os.MkdirTemp may return /var while Go resolves /private/var; mixing the two broke copied-example type projections and diagnostic rebasing.

trap: fresh private module paths change Go compiler action identities even with identical source. Private analysis prepends -trimpath to its captured GOFLAGS; caller flags remain last so explicit overrides survive. A compiler-export test compares two fresh roots and a literal tagged constant, rather than inferring reuse from elapsed time.

trap: prefix-only text rebasing corrupts private module-10 when module-1 is replaced first. Rebase complete path components in one pass; a workspace with fewer than eleven modules will not expose this. Default-enabled caches also require subprocess test suites to set a temporary SKGO_CACHE_DIR, including go generate and dev callers.

trap: Vite can transform restored source before Chokidar's awaitWriteFinish delivers its change. Updating the unchanged-file baseline from every transform then consumes that pending event; initialize from the first transform and advance from watcher delivery. A deterministic actual-plugin order test exposes this even when the real fswatch integration happens to pass.

trap: go generate injects GOFILE/GOLINE/GOPACKAGE and rewrites PATH. They describe the launcher rather than generation semantics; hashing them makes equivalent invocation styles evict the same target slot. Validate effective Go settings and resolved Node identity instead, while retaining actual frontend configuration inputs. Replay must also repeat Swift ownership preflight on the live destination, since private analysis deliberately omits old generated output.

trap: removing pnpm/action-setup alone leaves package-tarball tests depending on ambient pnpm. Route those pack operations through VP with the supported manager pin too. A new devEngines manager declaration adds its own lockfile document even when dependency versions are unchanged; preserve that generated manager lock for reproducible installs.
