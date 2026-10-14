# Development replacement planning

correction: The user rejects all incumbent development functionality, including its architecture, code and behavioral intentions. Delete it first; temporary absence of development support is intentional. Do not extend the old supervisor or preserve an optional native lane.
decision: Created GitHub issue #321 for total development-only deletion, independently executable from the replacement plan. Preserve failed worktree 73b6 at 360b874 and the external research cache.
doc_bug: Earlier research/planning instructions to extend the existing supervisor caused incompatible implementation direction. The new plan starts from #321's post-deletion repository and treats those instructions as superseded.
friction: Thread reads can include enormous nested command outputs even with bounded turn counts. Store responses and select user/final messages before printing; request includeOutputs=false for history orientation.
decision: Verified the planning worktree's frozen install through vp: Kit 3.0.0, Core 1.0.0 with bundled Vite 8.3.1. Its custom-environment hotUpdate dispatch exists without the raw-watcher workaround; gopls's concrete fsnotify implementation has a 500 ms diagnostics policy that must not become the dev-loop latency floor.
