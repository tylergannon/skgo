doc_bug: mission-prerender-lint.md names `$app/environment` as the source of `building`, but pinned kit 3.0.0-next.25 exports it from `$app/env`; the stale name passes Vite bundling yet fails the repository's sandboxed Svelte type-check.

friction: the mandatory sveltekit-current skill path is a dangling symlink because `/Users/tyler/src/sveltekit-adapter-go` no longer exists -> keep the Kit 3 facts available from a durable path under skgo.

friction: `pnpm test -- engine-build` still ran all 134 generated scenarios rather than filtering to the named feature -> use Playwright's generated-test filtering only after `bddgen`, or repair the package script before relying on this gesture as focused iteration.
