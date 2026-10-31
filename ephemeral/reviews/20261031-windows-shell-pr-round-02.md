# Windows shell independent review — round 02

Outcome: **no findings**.

## Target and authority

Re-reviewed the complete implementation and available proof in this worktree against `origin/main` (`b0b5f8bd1c8991b428b023b258249e0d64cd6324`), the repository/user instructions, the live bodies of [#441](https://github.com/tylergannon/skgo/issues/441), [#442](https://github.com/tylergannon/skgo/issues/442), [#443](https://github.com/tylergannon/skgo/issues/443), and [PR #447](https://github.com/tylergannon/skgo/pull/447). The implementation remains at `57a9e6e9812d5eda43b3623965ba653eb5ee0796`; the working copy additionally contains a 30-line README host-comparison/scope amendment. The PR fetched during this review still points at that commit and does not yet contain the amendment.

The explicit conversation acceptance remains the fresh `skgo new` → add Windows and Apple shells through `skgo add` → transfer → Windows build/run path without consumer source edits. The caller additionally supplied the authoritative current user request: get a PR up for the Windows shell changes, reviewed and merged. That fresh Windows path is demonstrated. These user requests do not require implementing Apple's navigation-origin feature. Issue requirements remain relevant to detecting omitted Windows behavior and overclaims, but referencing an issue without closing it does not make its entire independent feature scope part of this bounded user request. No caller instruction narrowed the defect search, and no files other than this artifact were edited.

## Findings and adjudication

No material findings or genuine nitpicks remain in the complete Windows PR as amended in the working copy.

### #441 remains open; its Apple/HTTPS acceptance is not a Windows PR defect

**Requirement:** #441 requires application configuration for additional Apple navigation origins, parsed scheme/effective-port matching, explicit redirect/new-window behavior, and a synthetic browser demonstration among two HTTPS origins. #443 references that navigation policy as a dependency.

**Evidence:** `plugins/native-shell/nativeapp/files/native/Sources/ApplicationWebView.swift:16–20` still permits only exact initial scheme/host/raw optional port equality. Its view API at lines 32–45 has no additional-origin input; `plugins/native-shell/nativeapp/config.go:21–32` has no origin-list configuration. Neither file has changed since round 01. Windows has a separate configured effective-origin policy in `plugins/windows-shell/windowsapp/files/windows/NavigationPolicy.cs:9–18`. The supplied Windows browser fixture redirects between HTTP loopback origins (`plugins/windows-shell/example/main.go:71–76`); the tagged C# consumer tests HTTPS URL classification, not real HTTPS browser navigation.

**Concrete impact:** A freshly added Apple shell cannot configure `https://identity.example/` in addition to `https://app.example/`; its existing guard cancels navigation to the second origin. Its raw-port comparison also differs from Windows' effective-port normalization. The requested shared policy meaning and real two-HTTPS-origin demonstration therefore remain unavailable.

**Current disposition:** The new README at lines 56–62 accurately says these requirements remain open, and PR #447 does not claim a macOS build or completion of #441. The live issue remains open. This meets the round-01 alternative remedy of explicitly leaving the corresponding cross-platform acceptance incomplete. It does not implement the capability, but the supplied user request is for the Windows PR and shell-addition coexistence, not Apple's new navigation feature. I accept that distinction and withdraw this as a material finding against the Windows PR. Keep #441 and its broader acceptance open; implementing it is required before claiming that acceptance complete.

## Round 01 disposition

The missing #442 host comparison is addressed in the current working copy (`plugins/windows-shell/README.md:34–54`). It compares UI/local logic, remote wire/models, credential authority, distribution and build ownership, explains the WinUI choice, and distinguishes integration assessment from runtime benchmarking. The repository-specific conclusion that a Tauri native client would need an SKGo protocol/model integration or bridge is an architectural inference from the existing C#/Zig client and Tauri's host model. Its platform premises match the primary [Tauri architecture](https://v2.tauri.app/concept/architecture/), [Tauri Windows installer](https://v2.tauri.app/distribute/windows-installer/) and [WinUI](https://learn.microsoft.com/en-us/windows/apps/winui/winui3/) documentation inspected in this review. No additional host implementation is required to make that bounded comparison useful. The amendment must be committed/pushed for the posted PR to contain this resolution.

The Apple/HTTPS gap is now disclosed clearly rather than completed; its issue-level status is retained above. The initial round treated that gap as material to the broad issue set; the authoritative current user request and explicit open-issue boundary establish why it is not a defect in this Windows PR. There is no further material finding in the Windows implementation or its demonstrated fresh-project path.

## Evidence and validation

This re-review retains the complete source and proof inspection from round 01 and checked the full PR diff against that source, the new README/worklog amendment, live unchanged issue bodies and PR description, and the still-current Apple and Windows navigation implementations. The implementation has not changed since the independently executed Windows-plugin package tests, tagged C# navigation consumer, exact scaffold/template comparison, live Windows verifier and proof inspection in round 01; rerunning expensive checks solely for a documentation amendment would add no behavior evidence. `git diff --check origin/main` passed again.

The accepted evidence is specific:

- The fresh project was created through the checkout CLI, both real plugins were installed, and `windows-app` and `native-app` were added through `skgo add`. The consumer has a versioned Go dependency and vendored sources, not a Mac-specific replacement directive.
- Its Windows authored files exactly matched current generator templates in the previous independent check. The Windows input verifier independently reported 1,604 unchanged transferred application inputs. The build/interactive UI transcript and inspected screenshot show the default generated page, literal Go query, write count `1`, and `Go recorded a greeting for Fresh Windows shell.` This supports the no-consumer-source-edits acceptance.
- The separate installed SDK-consumer MSIX proof records an authenticated native GET/POST/GET, `Confirmed: Windows native command`, a synthetic message, allowed HTTP-origin redirects/popup behavior, blocked unlisted navigation and independent message trust. The inspected logs name native Windows build 26200 and WebView2 156.0.4314.50; the development recipe names .NET SDK 10.0.403, winapp 0.7.1, WindowsAppSDK 2.6.4 and SDK BuildTools 10.0.28000.2957.
- The bundled child server serves the generated app's frontend and handlers; it does not impose a local Go server on client-only remote calls. The separate native HttpClient credentials and WebView destinations preserve that boundary. Server lifetime handling, create-only scaffold preservation and explicit native build exit checks remain as reviewed.
- Proof is for the checkout and existing Windows environment, not a newly published release, a macOS application run, real third-party authentication, full network payload capture, or completion of all three referenced issues. No consumer was reinstalled/relaunched in this re-review and the Windows worker/service state was not changed.

PR #447's bounded Windows claims are supported. This review does not certify completion of the explicitly open cross-platform acceptance above. Include the reviewed README amendment in the PR before treating this review as applying to its final head.
