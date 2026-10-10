# Existing VitePlus update

decision: VitePlus 1.0.0 documents a manual dependency update path in its shipped `docs/guide/upgrade-project.md`: align VitePlus, its core alias and bundled Vitest across declarations/catalogs/overrides, then install. Use that path for an application already declaring VitePlus. The actual staged `vp migrate` run formats source even for the existing recorder, so its documented upgrade-only behavior is insufficient for skgo's authored-source preservation promise. Keep guarded migration for first adoption.

correction: The primary pin is VitePlus. Read core and Vitest versions from the selected global CLI's `toolchain --global --json` graph; upstream Vite's version differs from its VitePlus core package version. pnpm remains the qualified companion.

friction: Creating package.json `pnpm.overrides` merely because another pnpm field exists suppresses unrelated workspace overrides. Keep the existing effective override owner; pnpm configuration alone is not an override map.

correction: The normal edited v0.27 recorder is `/private/tmp/skgo-recorder-consumer.GthvK4/edited-update-consumer`. Later explicit/full-migration diagnostic copies contain prerequisite patches and cannot qualify this repair. Preserve the original and run only on disposable copies. Development completion against v0.28 dependencies is not a released updater result.

correction: `@vitest/` is not one version family: browser-webdriverio and eslint-plugin release independently. Explicitly own the coupled Vitest siblings in both alignment and preservation checks; namespace-wide ownership silently rewrites unrelated choices.
