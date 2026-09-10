decision: Generated projects own a small Playwright-BDD acceptance contract; the release workflow runs it unchanged against development and production.
decision: Slow browser qualification runs after merge and tags the exact tested SHA only after success; pull requests retain fast checks.
decision: Conventional Commit type determines the automatic version increment, with breaking changes producing a major bump.
doc_bug: AGENTS.md requires /Users/tyler/src/skgo/ephemeral/inspiration/junkyard/.agents/skills/sveltekit-current/SKILL.md, but the junkyard directory is absent from the current pinned-source tree.
correction: Issues #123 and #124 were fixed on main by PRs #126 and #125 before this task began; this task preserves them as generated and release-level acceptance claims.
friction: Playwright-BDD generation cannot be compiled in-place from the embedded template because the source feature ends in .feature.tmpl; compile and execute it only after skgo new renders the actual .feature file.
decision: A release-only Go test is selected by the qualification build tag; ordinary PR tests contain no skipped browser scenario, while release qualification must explicitly compile and run it.
decision: The old local tag-push recipe and tag-triggered npm workflow are removed as bypasses; release-gate.yml is the only automated path that tags and then calls the trusted-publisher workflow.
