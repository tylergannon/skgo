# Native release preparation

friction: Gimbal releases currently invoke SKGO native build solely to obtain its generated Xcode project and native libraries, but that command also recompiles the frontend and an unused Debug app before Gimbal builds Release.

decision: Expose prepare-only and reuse-web-build options on native build. Preparation still generates native bindings and libraries and the Xcode project; it stops before Debug compilation. Frontend reuse requires a nonempty built manifest. The caller owns selecting outputs from the same source revision.

friction: Initial review found no regression tests for command omission or worker arguments. The same native build orchestration now accepts a command executor internally, allowing platform-independent tests of required native work, frontend/Debug omission, worker arguments and invalid prepared inputs. Native package tests pass in approximately 0.2s. Actual Gimbal preparation and subsequent unsigned Release build also passed; its bundled versioned CLI matched the supplied binary byte-for-byte.
