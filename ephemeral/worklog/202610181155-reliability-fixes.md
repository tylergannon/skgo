# Reliability fixes

- decision: The user requires one shared feature branch, ordered issue groups, independent token-cache annotation before implementation, and Claude Opus validation constrained to issue goals, load-bearing proof and reproducible introduced bugs. Avoid redundant full CI and new proof machinery.
- correction: #113's 2026-10-18 triage supersedes its historic goja render-lock instructions. Native Vite/Kit owns development rendering; keep the retained browser behavior regression.

- correction: #330 reproduces as a returned Mutex.Unlock R_ADDROFF relocation error, not a demonstrated stuck loader. Session fallback only recognized CompleteBuild, leaving old literal HTTP 200 indefinitely after that error; promote failed native installation through that existing fallback and never reuse an acknowledgement stream after a timed-out read.
- decision: The exact #330 mutex relocation diagnostic is a returned installation error; no blocked loader call was established. Reuse the existing complete-build fallback on native install failure and reject late acknowledgements after a bounded/cancelled control exchange.
- decision: #113 edits while actual Go I/O is held, releases, then requests the changed route. Holding until a complete replacement host is ready is distinct #349 work; do not infer a universal in-flight generation guarantee from Kit's dev manifest.
