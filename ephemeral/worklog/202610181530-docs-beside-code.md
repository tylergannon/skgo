# Docs beside code (skgo-project#186)

decision: guide pages live beside their code with frontmatter; code samples are empty fences naming a Go Example or a repo file; reference is godoc; the site extracts from the tag. Source: owner, skgo-project#186, 2026-10-18.
doc_bug: the brief estimated ~140 literal Go fences; the old tree had 141 Go fences of which 132 were `/// include:` of example/ files (about half regions) and only 9 literal. The real porting cost is the region includes, not literal Go.
friction: sample routes in example/web need `skgo generate`, whose outputs (link tree, skgo_gen.go, .remote.ts) are committed and shared across routes -> parallel writers each work in their own worktree and generated files are regenerated once after merging.
correction: writers turned literal Go fences into prose when no obvious compiled home existed; the contract requires an Example or a whole compiled file. Say "never prose" in the first brief, not after the first report.
trap: frontmatter `order` from the old two-digit page prefix ties every section at 10, so "sections order by their first page" cannot work; encoded as section*100+page (load = 2020) and flagged for the owner.
trap: an Example whose `// Output:` prints pinned versions is a drift check in disguise (fails on every pin bump); reference the constants file instead. Pins/Current were only used by the deleted docs package and were removed.
trap: sample routes with package-level state contradict the state-management page; key sample state by visitor.Of(ctx) like the example's own stores.
friction: zsh does not word-split unquoted $vars in `for f in $c`; run merge-resolution loops under bash.
- correction (owner): pages never paste command usage or output. A `text` fence whose info string is an skgo command line (`skgo new -h`) has an empty body and the site runs it in an empty directory. Trap for the site: `skgo build|buildinfo|env -h` exit 1 ("flag: help requested"), `skgo mcp -h` exits 2 and prints the top-level list, and every subcommand's help except `skgo plugin -h` goes to stderr.
