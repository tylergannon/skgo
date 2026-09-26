# What observation records do and do not exist in the probe directories
# origin: /tmp/skgo-jsdoc-probe-20260926, /tmp/skgo-jsdoc-demo-probe-20260926,
#         /tmp/skgo-jsdoc-jsproof-20260926
# retrieved: 2026-09-26 (reads only; no build or test was run)
# location: `find … -name '*.log' -o -name '*.png' -o -name '*output*' …` excluding
#           node_modules, plus `head` of each log found.
# --- verbatim ---

## 1. Every `.log`, screenshot-like, or test-output file found (excluding node_modules)

    /tmp/skgo-jsdoc-probe-20260926/web/debug-storybook.log
    /tmp/skgo-jsdoc-probe-20260926/web/static/robots.txt
    /tmp/skgo-jsdoc-probe-20260926/web/.svelte-kit/output/client/robots.txt
    /tmp/skgo-jsdoc-probe-20260926/web/build/client/robots.txt
    /tmp/skgo-jsdoc-probe-20260926/web/src/stories/assets/*.png   (Storybook artwork: testing.png, figma-plugin.png, …)

    /tmp/skgo-jsdoc-demo-probe-20260926/web/debug-storybook.log
    (same robots.txt / storybook-artwork shape; no other logs)

    /tmp/skgo-jsdoc-jsproof-20260926/web/debug-storybook.log
    (same shape; no other logs)

No `.png`/`.jpg` screenshot of a running page, no `test-results/`, no
Playwright `trace`, no `*.jsonl`, no captured `check`/`build` stdout, and no
`.tsbuildinfo`-style record was found in any of the three trees.

## 2. `web/debug-storybook.log` — what it actually records

    $ wc -c → probe 2954 bytes; demo 2994 bytes; jsproof 2994 bytes

First six lines of the demo copy (the jsproof copy is byte-identical in its
head; the probe copy differs only in directory name and timestamp):

    [10:54:04.844] [INFO] This command is running via an AI agent: codex. Proceeding with agentic installation flow.
    [10:54:04.866] [INFO] Initializing Storybook
    [10:54:04.868] [DEBUG] Getting package.json info for /private/tmp/skgo-jsdoc-demo-probe-20260926/web/package.json...
    [10:54:04.869] [INFO] Package manager: pnpm
    [10:54:04.869] [DEBUG] Getting CLI versions from NPM for storybook...
    [10:54:04.870] [DEBUG] Executing command: pnpm info storybook version

This is Storybook initialisation only. It records no remote-function probe, no
`svelte-check` result, no `vp build` result, and no adapter error.

## 3. App READMEs describe intent, not observed runs

`/tmp/skgo-jsdoc-demo-probe-20260926/README.md` lines 44-49 (verbatim):

    ## Go examples

    The home page calls the query and command in
    `web/src/routes/example.remote.go`. `skgo generate` creates the matching
    TypeScript module with throwing server stubs, so the displayed status and the
    write interaction can only be answered by Go.

`/tmp/skgo-jsdoc-probe-20260926/README.md` has the same shape (a template
README from `sv`/VitePlus). These describe what the apps are for; they are not
evidence that a command ran.

## 4. Summary of the evidence gap

Everything about *what previously ran and what it printed* in these directories
is either (a) a residue in build artifacts — which is what the other sources in
this topic capture — or (b) an operator report with no preserved exit status or
output. There is no file in any of the three directories that records a
svelte-check pass, a `record(123)` failure, or an adapter rejection message.
