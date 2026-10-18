# Tailwind live fixtures

Qualification is complete and the disposable `ephemeral/tailwind-live/` tree (798MB) has been removed. Generated scaffold source, dependencies and binaries are not retained. Concise actual new/add/repeat/generate/dev receipts are in `03-tailwind-live.log`; scanner output is retained as `03-tailwind-scan.json`. Independent parent browser observations below are unchanged.

The fresh app ran at `http://127.0.0.1:4171/` (coordinator PID 50419, initial Go PID 50778); the edited-existing app ran at `http://127.0.0.1:4172/` (coordinator PID 51491, initial Go PID 51840). Both current-source `skgo-live dev` runs used `GOWORK=off` because these nested disposable modules were intentionally outside the repository's go.work. Background watchers accompanied the dispatched processes.

The fresh fixture came from actual `skgo new` with local Go-module and adapter/add-on overrides. Its final generation needed GOWORK=off because it was nested under this repository. The existing fixture was a copy of that established scaffold with handwritten `web/src/styles/site.css`, root-layout import and project identity edited before actual `skgo add`. Both retained upstream VitePlus `lazyPlugins(() => [...])` configuration. Actual add-on application succeeded in both; repeat application reported `applied tailwindcss` with no `changed` paths.

Cleanup sent SIGINT only to the recorded coordinators 50419/51491. Both owning shell sessions exited 0. The recorded coordinators, Vite children 50705/51584 and final Go children 55278/55275 were absent afterward; neither public port had a listener, and both instance-private directories had been removed by their owners before deleting the fixture tree.

## Browser use and edit seams (executed before cleanup)

Each page exposes labeled `Unsent draft` input (`#draft`) and `Open second local view` button (`#open-view`). Enter an independently supplied literal draft and open the second view (`#second-view`, text `LOCAL_VIEW_350 is open`). Record the document lifetime independently (for example browser performance timeOrigin) and visible input/view state before each edit.

1. Root Go file: `APP/web/development.go`; change `ROOT_ONE_350` to `ROOT_TWO_350` in the function body. Root receipt is `#go-root`. For a structural full replacement control, also add an unexported package declaration such as `var rootStructural350 = true` in that file. Wait for the changed Go receipt before assessing unchanged document/draft/view.
2. Colocated route Go: `APP/web/src/routes/page.server.go`; independently change `ROUTE_ONE_350` to `ROUTE_TWO_350`. Route receipt is `#go-route`. A separate structural control can add `var routeStructural350 = true` there. Again wait for the literal new receipt, then check document/draft/view.
3. Frontend utility: `APP/web/src/routes/+page.svelte`; change `tracking-[0.1em]` to `tracking-[0.3em]`; inspect `#utility` computed letter spacing and visible change through normal HMR.
4. Authored CSS: existing stylesheet `APP/web/src/styles/site.css` already has `HANDWRITTEN_350` comment and `.handwritten` outline plus 7px letter spacing. Change that rule to 11px and inspect `#authored`. Fresh stylesheet is `APP/web/src/routes/layout.css`; append `.handwritten { outline: 3px solid rgb(19, 117, 83); letter-spacing: 9px; }` and inspect its effect. These frontend edits are separate from Go controls.

Both stylesheets received computed `@source not "../../**/*.go";` for their actual respective locations. Keep Go files visible to the application watcher. Do not introduce global Vite ignores, persistence, extra reload policy or special replacement hooks.

## Direct scanner observation

Fresh Tailwind is installed **4.3.3**. Its actual `@tailwindcss/vite/dist/index.mjs` uses `@tailwindcss/node.compile` with stylesheet dirname as base, combines its default frontend-root `**/*` with compiler sources, and constructs `@tailwindcss/oxide.Scanner`; `hotUpdate` full reload checks the scanner's files. An inline `vp exec node` invocation reproduced that same source construction for the selected CSS with and without the exclusion, without editing app files. `03-tailwind-scan.json` records four Go files before, zero after (including frontend-root `development.go` and colocated `page.server.go`), with the exact same 20 Svelte/TypeScript/HTML sources. This is scanner evidence, separate from required browser proof.

## Independent parent browser observations

The parent drove both real applications through CUA, separately from the implementation and fixture agents. Fresh draft was `DRAFT350 FRESH KEEP ME`; existing draft was `DRAFT350 EXISTING KEEP ME`. Both had `LOCAL_VIEW_350 is open` before any Go edits.

| Application | Document loader before/after all edits | Root Go save | Route Go save |
| --- | --- | --- | --- |
| Fresh | `6C8A3DCEBAF35ED07FE6C42A83EF0D88` unchanged | `ROOT_TWO_350`, original draft and open view retained | `ROUTE_TWO_350`, original draft and open view retained |
| Edited-existing | `C5FF20CE6B01858B95DDC792A640A4F8` unchanged | `ROOT_TWO_350`, original draft and open view retained | `ROUTE_TWO_350`, original draft and open view retained |

Each edit was a separate save, followed by waiting for its literal Go receipt in the browser. The native bridge refreshed Kit data after the Go replacement; no fixture refresh button or browser navigation was used. The dev logs show full Go host replacement for the root edit and subsequent successful data requests.

Frontend qualification then changed each utility class from `tracking-[0.1em]` to `tracking-[0.2em]`: computed letter spacing changed from 1.6px to 3.2px in both apps. Fresh CSS gained the supplied handwritten rule; existing CSS changed its supplied spacing from 7px to 9px. Both computed 9px spacing and `rgb(19, 117, 83) solid 3px` outline, and the existing handwritten rule/comment remained. Both document loader IDs still matched the originals. Svelte component HMR reinitialized the fixture's component-local state during its own source edit; state retention was asserted after each Go edit, before those frontend changes. Parent inspected both resulting screenshots: visible receipt values and styled targets, no error boundary or blank page.
