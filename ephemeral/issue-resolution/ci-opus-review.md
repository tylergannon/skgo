# CI reroute regression repair: PASS

Original failure (ci-first-build.log): `500 GET /about`, prerender_http_error, during the real example build.

Alignment:
- Pinned Kit 3.1.0 `respond.js:266` calls `hooks.reroute({url, fetch: event.fetch})` before `handle`, so the generated `handleFetch` (`requestFetch`) sees `/_app/reroute` before any logical request owner exists.
- Main ea1d75a4 answered that framework callback first in `requestFetch`, and `dev-bridge.js:629` does the same. The rebase had moved it inside #278's owner-bound `bridgeFetch` continuation.
- The repair restores main's order: same-origin check, exact `${base}/${appDir}/reroute` match, and authenticated `invokeService`.
- A non-matching path that only ends in `/reroute` now takes the ordinary bridge rather than main's bare `fetch(request)`. That keeps #278's fetch policy for application URLs.
- No product scope added.

Proof (ci-opus-focused.log):
- TestPrerenderRerouteFetchPrecedesLogicalRequest: PASS. It imports the shipped `skgo-adapter/prerender.js` and calls the real exported `requestFetch` with a continuation that throws. It asserts exactly one authenticated call to `/reroute` with literal URL, header, base and appDir inputs, plus the literal answer.
- Same test against HEAD's pre-fix `prerender.js`, via a /tmp copy loaded through a Go `-overlay` of the test file (no source edits): FAIL with `fetch-begin HTTP 409: no logical request exists before reroute`. The test is load-bearing for this ordering, which is the CI failure shape.
- example TestGoRerouteSelectsApplicationRoutes (5 subtests) and TestEveryPrerenderedPathIsServedFromItsFile: PASS, zero skips, against the existing repair build. `build/prerendered/about.html` (13:48:53) postdates the `prerender.js` fix (13:48:24) and matches ci-build-repair.log. The prerendered manifest lists `/about` and `/about/__data.json`, and the test serves each byte-for-byte from Kit's file. No rebuild.

Assessment: the new test protects real behaviour (#167 reroute during prerendering under #278's fetch bridge). It is meaningful and should stay; nothing here needs deleting.
