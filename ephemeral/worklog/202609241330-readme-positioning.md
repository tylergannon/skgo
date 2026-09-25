correction: first-draft pitch copy overclaimed ("all of SvelteKit", "nothing is reflected at runtime", "your Go types are your TypeScript types") -> every front-page claim is checked against source before it ships; a GPT-6 Astra review caught these by citing files.
decision: the headline is the plain claim "A Go backend for SvelteKit." on README, doc.go, docs/index.html and the GitHub About line -> change all four together; no slogan taglines.
decision: setup minutiae for humans' agents (Go Form client, Unix-socket transport) live only in skills/skgo/SKILL.md, never the README.
trap: the "Not supported" list exists in both README.md/docs/index.html and skills/skgo/SKILL.md -> when a limit is lifted, update all three.
trap: #81 (prerendering a page with a Go load) is closed NOT_PLANNED -> state the limit plainly; do not link it as tracked work.
decision: skills install through the skills CLI via `vp dlx skills add tylergannon/<repo> -y` (one repo per call; installs to .agents/skills and symlinks .claude/skills) -> the site and README prompt use vp dlx / pnpm dlx, never npx.
trap: headless Chrome --window-size below ~500px still lays out wider -> check phone width by screenshotting the page inside a 390px iframe.
