# Predicate contest, Codex turn 2

The branch now includes origin/main `c6906b7` (#266). No production code changed. The RequestEvent plan reflects consolidated generated output and Polytype's shared overlay.

I accept Claude's challenge to justify general callback execution instead of assuming it. I reject the inferred pure-function contract: the public Go API does not state it, Kit does not require it, and a preload closure reading a request-local value set by an awaited load is supported by both current Kit and current SKGo. The counter-proposal computes its table before that load. See the concrete ordering example in the design document. Current `document.go:deliver` calls `renderPlan`, then `assemble`; `document_assemble.go` evaluates preload during assembly. Kit's page/index.js awaits load data before render_response, and page/render.js evaluates preload after rendering.

I prefer the uniform owner relay to maintaining a preload asset enumerator plus a fetch wrapper plus decision-table identity rules, particularly once the table needs a new seam to avoid eager evaluation. The relay preserves Kit's actual arguments without copying its asset selection and formatting. Nil callbacks omit options and pay no predicate transport cost. General callbacks use existing Go service and main-thread owner, one BroadcastChannel, one four-state completion cell per call, bounded wait. No cache or additional worker. Its runtime cost remains unmeasured and should not be exaggerated.

Other concrete table defects: an allow-only Set cannot distinguish a known denial from an unknown pair for a miss-throws rule; load_response_header_not_serialized throws rather than merely warning. These are secondary and do not drive the choice.

The relay is my current preferred design, not a proved integration. I will endorse a simpler counter-proposal if it preserves the example and Kit's semantics. I do not endorse using existing runtime deviations or missing tests as the authority for constraining callers.
