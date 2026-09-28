# Kit environment integration

correction: Kit 3.0.0-next.28 does not prohibit dynamic-public reads during prerendering. Its generated rendered_env has plain assignments; prerendered browser bootstrap fetches env.js before evaluating app modules. Build HTML can consequently reflect build values while the browser reads startup values.

friction: Kit loads env.ts in a standalone Vite server with configFile:false, so an adapter virtual module cannot provide its validated build inputs. Use a server-only generated filesystem module, with explicit development HTTP denial.

security: The existing development module transport could return transformed server modules to public callers, including newly introduced static-private literals. Environment-enabled Vite instances now require a token stored only in the local development snapshot; the Go browser proxy refuses bridge routes. Vite filesystem denial also covers raw snapshot and server artifact paths.

contract: Kit emits export const NAME = env.NAME, so assigning environment after loading application modules is too late. Initialize each renderer's own snapshot before module evaluation; Go validation does not execute in renderer runtimes.

security: Live verification found Vite ?raw still served generated config/private modules despite additions to configResolved fs.deny. Those late matcher changes were insufficient. The adapter now checks resolved file paths in middleware before Vite transforms, with ordinary/raw/import and /@fs regression cases. Connect's /module mount also accepts trailing segments, so authentication covers that entire mount, not only its exact URL.

security: Mac's case-insensitive filesystem also resolves .SVELTE-KIT to .svelte-kit, bypassing lexical path checks. Node realpathSync.native canonicalizes disk casing whereas realpathSync does not on this host. Canonical native paths now protect mixed-case and symlink aliases, with a real-filesystem regression.
