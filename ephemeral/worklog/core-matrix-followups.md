correction: The requested issue numbers span repositories: skgo-project #105/#106/#107 are active matrix validation tasks; skgo #105/#106/#107 are unrelated closed historical bugs. skgo #223 is the active failed-prerender diagnostic requirement. Verified live GitHub issue bodies and comments before dispatch.

finding: The retained native query counterexample uses Kit next.28 and an unretained proxy. PR237 f572a58's items universal loader now returns data.item, keeping a proxy reference. Treat the current retained-reference single-flight contract separately from an unretained functional-load claim; do not blindly weaken its count assertion.

source: Stable Kit 3.0.0 package/version and cache, query proxy and prerender sources were read at /Users/tyler/Codex/2026-10-02/task-18/skgo-js-dependencies/example/web/node_modules/@sveltejs/kit. Repository source notes still point at next.28; that path is not authority for this follow-up.

decision: Follow-ups use isolated task-8 worktrees based on PR237 f572a58. Release owner retains PR237/tag ownership; rebase and qualify after its merge. Three bounded Luna workers own query validation, independent source-edit invocation, and authored Go prerender diagnostics, respectively. No site files or dependencies are shared mutation targets.
