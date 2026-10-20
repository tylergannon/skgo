# Tiny native application

This is the native-shell release example: one Go value, one generated Swift
query and one generated Swift command, displayed inside the existing Apple
shell. Launching the built executable with one argument prints the actual query
result before and after the command and exits. Without an argument it opens the
application normally. Expected values and release assertions belong to the
separate qualification job.

Use the official skgo executable targeted by the native-shell artifact. Prepare
this checkout for that exact release with the existing updater completion
operation below: replace `VERSION` with the core version from the artifact name.
That one operation selects the official executable's compatible adapter and
frontend pins, regenerates the lockfile through VitePlus, updates the Go module
and generates/builds the web application. It replaces the checkout-local adapter
link; no source editing or adapter-version guessing is required. Run it from
this directory before installing the plugin. Disable the parent core workspace
when working in this checkout.

```sh
export GOWORK=off
skgo update --complete-version VERSION --root .
skgo plugin install --prebuilt ARTIFACT --sha256 PUBLISHED_SHA256 \
  --select 'src/routes/value.remote.ts#readValue' \
  --select 'src/routes/value.remote.ts#writeValue'
skgo plugin native-shell build --platform macos
native/build/macos/DerivedData/Build/Products/Debug/TinyNative.app/Contents/MacOS/TinyNative YOUR_VALUE
```

The shell is already application-owned source here; do not run setup again.
Generated Swift, native build output and the installed artifact are disposable.
The example deliberately has no local skgo replacement. The updater and native
build use released dependencies; a development checkout is not an official
release pairing.
