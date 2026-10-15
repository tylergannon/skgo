# SSR response cookies over Go UDS

The missing outer-response cookies are a known, accepted, non-blocking limitation of the narrow UDS prototype. The topology can preserve them through supported Kit APIs. This is installed Kit **3.0.0** source assessment, not a new runtime result. Retrieval started at token-cache `index/dev-native-kit-uds.md`; no product files changed.

## Recommendation

For the direct Go UDS fetch hook, consume each `response.headers.getSetCookie()` **inside the server hook**, parse it with `event.cookies.parse`, then register it using `event.cookies.set` before returning the response. Kit explicitly documents this external-response-cookie use case in installed `types/index.d.ts:369-391`, including the exact parse/set loop. No Kit internal mutation or serialized-header allowance is needed.

Use the individual headers, never split a comma-joined header: Expires contains commas. Restrict this to the intended same-origin Go application delegation. Do not relay arbitrary third-party response cookies into the application's cookie jar.

For faithful existing wire semantics rather than Kit defaults, supply explicit effective attributes. `runtime/server/cookie.js:74-79,161-162` adds defaults (HttpOnly, Path=/, SameSite=lax, production-dependent Secure). A raw Go cookie without those attributes must not silently acquire them if fidelity is claimed. Parse with identity decode and preserve value bytes with identity encode, as Kit's own internal-fetch path does (`runtime/server/fetch.js:152-167`). Resolve a missing Path using the fetched URL's directory, not the outer page path. Carry supplied Domain, Expires, Max-Age, SameSite, Secure, HttpOnly, Partitioned and Priority; override absent boolean/default attributes appropriately rather than relying on defaults. Distinguish a deliberate application policy of adopting Kit defaults from byte-faithful relay. Unknown/nonstandard attributes are not guaranteed by the supported parser/serializer and need no invented compatibility claim.

An alternative naturally preserves Kit's own internal-fetch rules: let `handleFetch` call its supplied `fetch(request)`, have native Kit dispatch to a generated `+server` callback that delegates execution to Go UDS and returns Go's Response. Then `runtime/server/fetch.js:150-170` automatically parses returned Set-Cookie into the outer request's internal cookie state. This fits the broader generated endpoint bridge, but requires that bridge; the public parse/set API is the smaller addition to the already working direct-fetch prototype. Neither option requires a custom JS cookie store.

## Four distinct observations

| Claim | Source/result boundary |
|---|---|
| Go Set-Cookie is readable inside server `handleFetch` | Node Response retains header list; current UDS hook appends raw Go headers. It can inspect `getSetCookie` before universal-load response wrapping. Prior direct Go proxy proof established two separate headers, not outer SSR propagation. |
| Later server `event.cookies` and ordinary Kit fetch see it | `cookies.set` writes the request's `new_cookies` map (`cookie.js:228-243`); `get/getAll` read matching new cookies (`:91-142`); default fetch uses `get_cookie_header` where pending cookies override inbound cookies and explicit Cookie overrides both (`:189-222`). A **second custom UDS hook still copying only incoming event.request Cookie will remain stale**: it must use matching pending cookie state too, or use default continuation. Registering response cookies alone fixes neither custom outbound scope nor credentials. |
| Browser receives outer SSR Set-Cookie | `respond.js:461,518` calls `add_cookies_to_headers` with the request's new cookies; `cookie.js:306` emits separate Set-Cookie headers. Registration must occur before that response is finalized. Cookies arriving after streaming headers are sent cannot be added retroactively. |
| Universal load can read/serialize those headers | Different rule. `page/load_data.js:462-496` guards both `headers.get` and `getSetCookie` with `filterSerializedResponseHeaders` when CSR is enabled. Reading in the server hook avoids this universal-load guard; it does not authorize exposing cookies in HTML. Do **not** whitelist Set-Cookie to make relay work or place HttpOnly values in returned page data. |

The bare POC returns its Response early from `handleFetch`, bypassing the default fetch's `set_internal` loop. Therefore readable subresponse headers do not imply pending-cookie updates or browser receipt. Prior runtime finding: `../20261015-vite-uds-ssr/REPORT.md`, request/cookie boundary, explicitly observed missing outer SSR Set-Cookie. This investigation ran no new runtime test and does not upgrade that result.

## Small unrun qualification

One isolated page can establish the remaining integration: Go returns two independently supplied cookies (one HttpOnly, one with explicit scoped Path and expiry); server hook records them through parse/set; the next same-request server fetch reads the expected matching cookie; outer response contains both separate Set-Cookie headers with expected attributes; subsequent browser request sends the intended cookie. Assert an independent nonsecret rendered literal and confirm no cookie value appears in page data. Include an explicit-Cookie request and `credentials:'omit'` to expose accidental unconditional forwarding. This is ordinary handler/SSR transport qualification, not a browser benchmark. No running processes were started for this source assessment.

Actionable worklog: Kit 3 already added `cookies.parse` specifically for external response relay; use it rather than importing cookie internals or inventing a parser. Its documented parse/set example adopts Kit defaults, whereas exact relay of preexisting Go cookie attributes needs explicit handling. Pending inbound state and outbound fetch cookie construction are separate fixes.

## Is an upstream change warranted?

**Recommend a documentation clarification first, not an automatic behavior change.** Installed `types/index.d.ts:1321-1328` says handleFetch can modify or replace an event.fetch result; it does not promise that every Response returned by the hook is postprocessed into the outer cookie jar. Current [official hooks documentation](https://svelte.dev/docs/kit/hooks#handleFetch), checked 2026-10-15, describes replacement and request credential forwarding but contains no Set-Cookie propagation promise. It is reasonable to find the transport-replacement asymmetry surprising, but this evidence does not establish a broken documented response-cookie contract.

The source distinction is precise: `runtime/server/fetch.js:31` directly returns `hooks.handleFetch(...)`; only the supplied continuation's **internal route** branch performs the Set-Cookie loop (`:150-170`). Its external-network branch returns global fetch directly (`:80`). Therefore even a hook that calls the continuation with a rewritten external backend URL does not automatically get internal-route response-cookie adoption. The docs' backend URL rewrite example does not claim otherwise.

The existing public `cookies.parse` / `cookies.set` example explicitly covers adopting external response cookies, so SKGO does not need an upstream change to proceed. Ask upstream to document that short-circuiting or redirecting fetch transport bypasses internal-route cookie adoption, cross-link that existing example, and distinguish request credential forwarding from response-cookie adoption. That is a small non-breaking improvement with a concrete surprising use case.

If upstream wants to support transparent alternate transports beyond documentation, the smallest useful follow-up is an **opt-in continuation/transport seam retaining the existing Kit request and response processing**, rather than a new broad cookie API. The integration need is to replace only the actual request transport while retaining native scope/pending-cookie rules. Exact API shape belongs upstream and is not designed here; a response-only helper would solve adoption but not stale outbound pending cookies. This is optional ergonomics, not an architecture prerequisite.

Moving the existing loop unconditionally outside handleFetch has compatibility hazards:

- Third-party or deliberately synthetic hook Responses would suddenly mutate the application cookie jar. Today external fetch results are not adopted automatically. A hook may intentionally return or inspect Set-Cookie without sending it to the browser.
- Hooks that already call the continuation or `cookies.set` would be processed twice. The cookie map can deduplicate identical domain/path/name tuples, but this does not prove equivalent ordering when hooks replace, delete or rewrite cookies after the continuation returns.
- Cookie Path defaults depend on the **actual fetched URL**, which may differ after rewriting. The outer hook call only retains the original URL unless transport provenance is carried explicitly. Relative/default paths and domain handling could change scope.
- A blanket rule must decide same-origin versus cross-origin, credentials omit and redirect provenance. The current internal branch's adoption behavior must be mapped before asserting a generic browser credentials policy; blindly adding a new global omit check or copying all cookies would each change existing behavior.

An opt-in preserves today's callers and makes ownership deliberate. Do not solve this by relaxing universal-load serialized-header restrictions: server cookie relay and client-visible serialized headers remain separate contracts. No upstream issue/comment was posted and no new runtime campaign ran.

Actionable correction: 'the default fetch handles response cookies' is too broad. Only its internal-route branch does; external continuation requests do not. This sharpens the upstream documentation question without making it a blocker.
