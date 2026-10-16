# Controlled overlap observations

Experimental input: current preserved coordinator, native host and adapter; no production edits. Private overlay adds only named stage barriers and records observed Project revisions. Baseline uses a private copy of the actual example and its actual `NewHandler`; Vite/Kit pins and generated remote stubs are unchanged. Public57131/control57132. `GOWORK=off` required so the enclosing workspace cannot select the original example module (initial failed attempt retained).

Independently supplied literals: baseline `FRONT_A_2101` and `GO_A_1101`; superseded Go `GO_B_1202`, current Go `GO_C_1303`; combined frontend `FRONT_B_2202` and Go `GO_D_1404`; new route `/aaa-overlap` contains `ROUTE_MEMBER_5101` and remote `REMOTE_MEMBER_6101`. Browser draft supplied `OVERLAP_DRAFT_8201`, Count1.

Baseline actual browser: FRONT_A_2101, GO_A_1101, draft8201, Count1. CDP timeOrigin1792101433591.5. Actual WebSocket request96588.185 used `ws://127.0.0.1:57131/__skgo/dev/hmr?token=...`, HTTP101, subprotocolvite-hmr, public Origin57131. DOM element presence alone is not hydration proof; successful real input and counter interaction supplies that proof. Initial browser evaluation could not read performance in restricted DOM scope; CDP Runtime read supplied timeOrigin. No page state was injected.

Latency agent requested serialization: baseline process stays idle until its measurement window completes. No overlap candidates have yet been submitted at this checkpoint.

Predeclared additional notification-boundary literals: `GO_E_1505` is validly installed and held immediately before notification; `GO_F_1606` is written and must be observed as a newer Project revision before releasing that older notification. This tests notification ordering separately from stale candidate installation. Structural overlap changes the existing page to `GO_G_1707`, then adds the supplied member route while an older captured candidate is held; recovery must return that exact current Go literal plus the new route's independently supplied remote6101. These expectations precede submitting these edits.

## Go supersession

Held captured revision4 containing supplied GO_B_1202. Wrote GO_C_1303 and explicitly read control state `{held:captured,revision:5}` BEFORE releasing revision4. During hold real remote returned baseline GO_A_1101. After release browser showed GO_C_1303 with original draft/Count1; actual public WS96588.185 emitted ready revision5 twice (CDP sequences697/716), no revision4 in the captured frame window695→734. Old revision4 never reached before-notify in barrier log. `go-held.json` and `go-current.json` preserve actual remote responses. This proves this controlled observed supersession; it says nothing about an unobserved file write.
