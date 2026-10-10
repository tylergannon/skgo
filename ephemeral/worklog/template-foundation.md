# Template foundation

decision: Execution supersedes the planning-only scope. The approved contract is skgo-project 4f04eaf, with shared implementation tracked by skgo #293.
correction: The user now wants the canonical recorder source in a new skgo-template-voice-recorder repository. Its peer owns recorder extraction; skgo owns the native foundation. Keep the original Voice Notes checkout intact.
friction: skgo root contains unrelated local deletions and untracked work. Work from codex/template-foundation; preserve the root state during fast-forward sync.
friction: Go 1.27 records a VCS-derived version for a local checkout binary (v0.26.1+dirty here), not necessarily (devel). External plugin builds must use buildinfo from the actual executable, including during local qualification.
friction: Local Go 1.27.2 export data is newer than the pinned Staticcheck reader; the repository's Go 1.27.1 toolchain passes its analyzer checks. Go 1.27.1 plugins on this macOS also exposed malformed chained-fixup metadata; use the external linker's no_fixup_chains flag for Darwin plugin linking and qualify the result in its exact host before installation.
friction: Fresh application qualification under this worktree requires GOWORK=off to avoid inheriting the parent workspace, and the pinned VitePlus environment with its corresponding pnpm version. Plain system vp/pnpm are newer here.
friction: A bare WKWebView in the generated macOS SwiftUI slot compiled and served valid HTML but displayed a blank window. The native shell must give its content an explicit expanding frame; independent screenshot and accessibility inspection then showed the welcome heading and documentation link.
decision: NativeHost retains the Go handler and loopback origin through listener recovery. ApplicationState owns recorder lifetime and bridges host lifecycle through callbacks; only the application WebView bridge may certify transcript display readiness.
