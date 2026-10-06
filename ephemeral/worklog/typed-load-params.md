decision: User's final scope is loads only; remote command/form argument APIs and caller propagation are a subsequent change after this milestone lands.
friction: Fresh managed worktree has no installed Kit source -> install each worktree's frozen dependencies before dispatch; verified installed Kit 3.0.0.
decision: Installed implement workflow's qa-orchestration session performs independent validation; coding gpt-6.1-sol:high and QA claude-opus-5-5:high, leaving planner/critique defaults intact.
friction: Installed Gimbal daemon-status probe errors when Codex's socket is absent instead of auto-starting -> start the installed Codex app-server daemon explicitly, then redispatch the untouched outcome.
friction: Concurrent full Go tests and source-edit BDD race transient Playwright artifacts during fixture copying and invalidate embedded-build freshness -> finish BDD, rebuild restored sources, then run Go tests against stable sources.
correction: Independent Opus runtime validation caught eager typed-param construction reporting unread fields as dependencies; preserve actual runtime reads, not all declared fields or static potential reads.
correction: Kit layout params include optional child-page params from actual layout membership; a directory's own RouteParams alone is insufficient for layout loads.
