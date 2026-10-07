# RequestEvent developer API and delivery limits

Application code owns an editable `Locals` struct in the configured locals
package. `--locals-package` selects that package; `--locals-type` defaults to
`Locals`. A hook selected with `--hook-package` / `--hook-symbol` receives the
shared typed event and explicitly forwards it with `resolve(ctx, event,
options...)`. Replacing `event.Locals` selects the object downstream only when
that event is forwarded. `app.LocalsFrom(ctx)` and derived events read the
selected pointer. Internal fetch starts another request with fresh locals;
nested remote invocation retains the current request's object.

Generated page and layout aliases are distinct: `PageRequestEvent` binds
`RouteParams`; `LayoutRequestEvent` binds `LayoutParams`. Layout domains come
from Kit's participating page graph, including resets and groups. Descendant
keys are optional, and matcher alternatives retain their actual Go types.
The shared `params.RequestEvent` binds the application-wide parameter domain.
Generated declarations live in `skgo_gen.go`; authored locals and hooks remain
editable across regeneration.

`Request()` retains the transport request. `URL()` and `SearchParam()` use the
logical URL; `RouteID()` reports the matched route and tracks a dependency
within loads. Parameter accessors track only the reading load; `Untrack`
suppresses those implicit dependencies. Request-kind flags and the lazy
`ClientAddress()` hosting provider survive event derivation. Internal fetch
inherits the address result but creates fresh locals. Build requests have no
client address. Queries still refuse caller route, params and URL access,
including access through nested context helpers.

Runtime and prerender hooks share value-event forwarding and resolve options.
The generated build entry binds actual hooks, loads, remotes, matchers and
endpoints. A logical prerender request owns the locals across its callbacks and
wraps Kit's returned response; completion and failures drain its state. HTML
transforms, serialized-header filtering and preload decisions use the selected
request state. Predicate decisions are synchronous booleans in Kit, relayed by
the existing build owner without another worker.

The integration proof uses the ordinary generated entry under strict Kit errors
for lifecycle/endpoints/locals and hook-selected options. A separate explicit
default-sharing fixture keeps its intentional rejected-header page under a
permissive policy. The example's `/event-layout/...` demonstrates selected
locals through hydration and layout reuse/reruns for parameter, route and
untracked reads. Fresh custom-module starter contracts cover authored-file
preservation, both internal fetch entries, action/command/form/query refresh,
and the no-hook path; an actual CLI-created scaffold separately exercises
edited domain locals and served document/data/endpoint behavior.

Accepted limitations remain and narrow the original ideal definition of done:

- [#272](https://github.com/tylergannon/skgo/issues/272): own-page/data fetching
  during prerender is unsupported; Go-backed endpoint fetching is supported.
- [#275](https://github.com/tylergannon/skgo/issues/275): a filter configured only
  in runtime `SSROptions` is not automatically imported into the generated build
  entry. Put shared defaults in the common hook's `ResolveOptions`, or supply
  an authored adapter `prerenderPackage` using
  `PrerenderServiceOptions.FilterSerializedResponseHeaders` explicitly.
- [#276](https://github.com/tylergannon/skgo/issues/276): an application Go HTML
  transform error rejects the whole build even when Kit's `handleHttpError`
  would ignore a failed page. The real permissive-build contract demonstrates
  this stricter policy and owner cleanup.

Independent whole-feature Opus review found no deployment blocker. Validation
on merged `main` remains a delivery step; branch proof is not main proof.

- [#278](https://github.com/tylergannon/skgo/issues/278): runtime `HandleFetch`
  and `HandleError` configuration is not carried into prerendering. Selected
  request middleware is shared; fetch rewriting and error shaping are not.
- [#277](https://github.com/tylergannon/skgo/issues/277): new route readiness
  measured about 59 seconds locally. Main CI comparison suggests added overhead,
  but it is not a controlled timing comparison and the cause remains unproven.
