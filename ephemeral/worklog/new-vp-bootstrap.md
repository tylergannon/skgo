# Fresh creation toolchain

correction: Issue #300 is ordinary base creation, before template application. Qualified vp 1.0.0 can still choose pnpm latest; checking vp alone does not establish its companion environment. Keep vp primary and use its environment selection for all creation children and its resolved pnpm for in-process Kit patching.
decision: This branch is independent of installer #299, updater #297, and native runtime acceptance. The actual creation probe uses the ordinary CLI with an installed qualified vp on PATH and conflicting ambient companion settings; no external wrapper selects pnpm.

friction: Running vp create inside vp env exec alone still chose pnpm latest and persisted it in devEngines. Installed vp 1.0.0 create source reads the enclosing project declaration or falls back to latest. A temporary root packageManager declaration makes upstream select and persist the companion correctly; it is removed on success and failure.
correction: The first fixture put managed pnpm on ambient PATH, hiding removal of the environment wrapper. Independent validation separated those paths and proved wrapper, Create wiring, metadata and cleanup omissions fail behavioral assertions.
