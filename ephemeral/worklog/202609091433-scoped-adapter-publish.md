correction: npm only permitted a bypass-2FA bootstrap token with the skgo organization scope. The previously selected unscoped public package name was not worth blocking release on.
decision: Publish the adapter as @skgo/sveltekit-adapter. This is an authorization namespace mandated by npm's available secure publish path, not a forecast that skgo needs a package family.
