# Windows shell independent review — round 01

Outcome: **material findings remain**.

## Target and authority

Reviewed the complete 24-file change from `origin/main` (`b0b5f8bd1c8991b428b023b258249e0d64cd6324`) to `57a9e6e9812d5eda43b3623965ba653eb5ee0796`, surrounding Apple/native-client implementation, the repository and user-supplied AGENTS instructions, and the live bodies of [#441](https://github.com/tylergannon/skgo/issues/441), [#442](https://github.com/tylergannon/skgo/issues/442), and [#443](https://github.com/tylergannon/skgo/issues/443). The additional conversation requirement is a fresh `skgo new` application with both shells added using `skgo add`, transferred to the existing Windows VM, built and run without consumer source edits.

The read-only boundary and requested artifact location were honored. No caller instruction narrowed the defect search. The findings below include an unmet cross-platform issue requirement even though it is outside the changed Windows files; it must not be confused with a newly introduced Apple regression.

## Findings

### 1. Issue — #441's Apple origin configuration remains unimplemented

**Requirement:** #441 specifically starts from the generated Apple shell and requires an application-configured list of allowed navigation origins, parsed scheme/effective-port matching, defined redirect/new-window behavior, and the same policy meaning on Windows. #443 explicitly depends on that policy.

**Evidence:** `plugins/native-shell/nativeapp/files/native/Sources/ApplicationWebView.swift:16–20` still allows navigation only when the destination's scheme, host and raw optional port equal the initial URL. The view exposes only `url` and `recoveryRevision` at lines 32–45. `plugins/native-shell/nativeapp/config.go:21–32` has no navigation-origin configuration. Neither Apple file changes in this branch. In contrast, Windows `NavigationPolicy.cs:9–18` accepts additional origins and normalizes the effective port.

**Reproduction/impact:** Generate the Apple shell with the same `skgo add ... native-app` path used in the fresh dual-shell proof. Its emitted `ApplicationWebView.swift` has no list to configure. A navigation from `https://app.example/` to a second intended origin such as `https://identity.example/` reaches the existing guard and is cancelled. Even `https://app.example/` versus explicit `https://app.example:443/` has different raw optional ports in the Apple policy, while Windows compares the effective port. Thus the two generated shells cannot carry the policy with the meaning required by #441. Successful Windows navigation does not fulfill the Apple requirement.

Implement and qualify the Apple configuration/policy while preserving application-owned files, or explicitly leave #441 and the corresponding cross-platform acceptance incomplete. The supplied live navigation fixture also uses HTTP loopback origins (`plugins/windows-shell/example/main.go:71–76`), so it is not the two-HTTPS-origin browser demonstration required by #441; the separate C# policy test proves classification of HTTPS strings only.

### 2. Issue — #442's required host comparison is absent from the development recipe

**Requirement:** #442 requires evaluating an SKGo–Tauri boundary alongside the C#/.NET/WinUI candidate, including remote wire format, generated bindings, authentication and distribution, before treating the selected Windows implementation as ready.

**Evidence:** `ephemeral/worklog/20261031-windows-native-shell.md:1` records WinUI/WebView2 as the decision, and `plugins/windows-shell/README.md:1–5,66–106` documents that implementation and its ownership. There is no comparison with Tauri in the recipe, SDK guide, native-shell guide, worklogs, or other tracked Markdown inspected; a repository text search for `Tauri` returned no matches.

**Impact:** The implementation has a coherent and demonstrated C#/WinUI boundary, but the explicit decision input requested by #442 remains missing. Readers cannot assess the requested alternative or see which wire/client/authentication/distribution work would be shared versus newly required. This is a missing design deliverable, not evidence that Tauri is preferable or that two hosts should be built. Add the bounded comparison and rationale, or leave that part of #442 open.

## Evidence inspected and independent checks

- Read all changed implementation, templates, project/manifest/build files, tests and documentation; examined the existing C# transport/client and Apple scaffold/configuration.
- Read both supplied worklogs; fresh creation, shell-addition, preparation, repository-test and Windows UI transcripts; the transfer bundler, command list and generated consumer. The source uses a private versioned Go-module proxy for the unreleased checkout and a vendored consumer, with no local Go replacement in its `go.mod`. This establishes the source branch path, not availability of a published release.
- Independently ran `go test -count=1 ./plugins/windows-shell/...`: all packages passed, no skips. Ran `go test -tags skgo_qualification -count=1 -v ./plugins/windows-shell/windowsapp -run TestCSharpNavigationPolicy`: the actual C# consumer passed origin normalization, default restrictions, lookalike/scheme/port rejection and independent message trust. `git diff --check origin/main` passed.
- Compared every non-template authored Windows scaffold file in `.local/fresh-dual-shell-e2e/application/windows/` with the current generator input, substituting only the chosen package identity: exact equality. Read the resolved generated host and both generated-shell trees.
- Used the documented SSH route directly to the existing Windows 11 guest. Reran its existing input verifier: **1604 unchanged application inputs**. Read its `launch.log`, which records the .NET shell build and actual shell process launch, and its UI transcript showing the bundled application's dynamic loopback page, literal Go-query text, write count `1`, and `Go recorded a greeting for Fresh Windows shell.` Inspected `ephemeral/screenshots/fresh-dual-shell-confirmed.png`; it visibly shows those results in the Windows shell. This supports the user's fresh creation/add-both/transfer/build/run path without consumer source edits. It does not claim a macOS app was built or run.
- Read the separate synthetic SDK consumer's Windows `install-final.log`, `final-shell-proof.log`, `live-final-check.log`, and `server.err.log` under `C:\gimbal\workspaces\skgo-windows-shell\.local\windows-example`. They record installed package `SKGo.WindowsExample_1.0.0.0_x64__1m9tmap03wzjw`, status `Ok`, a native authenticated GET/POST/GET sequence, native confirmed value `Windows native command`, synthetic native message delivery, blocked unlisted origin, allowed second-origin/popup flow, and WebView2 `156.0.4314.50`. The transcripts identify native Windows build `26200`; the runbook identifies Windows 11 Pro 25H2. The project pins .NET target 10, WindowsAppSDK 2.6.4 and SDK BuildTools 10.0.28000.2957; recipe-tested .NET SDK is 10.0.403 and winapp 0.7.1.
- The current review inspected prior interactive UI proof and read live transferred inputs; it did not independently reinstall or relaunch either consumer. No SKGoShell process was returned by the live process query. The `gimbal-worker` service remained Running, and no service, credentials, application source or proof input was modified.

No additional material Windows implementation defect was established. In particular, the bundled server in the default generated app does not contradict the client-only requirement: the synthetic native client uses its own HttpClient/portable DLL against a separate API destination, while the default Go child exists to serve the generated application's own frontend and handlers. Navigation and native message authority are separate, and inspection omissions and application-owned distribution/update duties are documented.

The demonstrated fresh Windows path is credible. The overall three-issue acceptance remains incomplete for the two explicit requirements above.
