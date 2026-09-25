# Browser suite in parallel, state per visitor

trap: `example/internal/skgo/links/**` are copies of the route packages, not
symlinks. Editing `web/src/routes/**/*.go` and running `go build` compiles the
old copy and passes. Run `just generate` after any route Go edit.

trap: minting a visitor id on every cookie-less request breaks
`TestALiveQueryRendersItsFirstValueIntoTheDocument` (a cookie-less write, then a
cookie-less document that must show it). Only a request with `Sec-Fetch-Site`
(a browser) is handed an id; everything else stays on `businesslogic.Default`.

trap: the root layout load's cookie write is visible to the same document's SSR
remote calls only through a request local — SSR remote calls build their event
from the request, not from the load's jar.

finding: skgo's cookie jar was unlocked while kit runs a branch's loads
concurrently. Two loads writing cookies (root layout visitor id + Actions
workspace) killed the server with `concurrent map writes`. Fixed in event.go;
`TestTwoLoadsOfOneBranchMayWriteCookiesAtOnce` crashes without the lock.

finding: kit runs a skipped parent server load when a child calls `parent()`
(`runtime/server/data/index.js`), so /account → Orders → Overview → refresh is
serial 3, not 2. The old "serial has changed" assertion hid this.
