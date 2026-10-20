# Universal-load fetch of imported assets (SSR)

- The goja environment overrode Kit's `assetFileNames` with
  `assets/[name][extname]` since #80, so every `?url`/asset import the engine
  rendered or fetched was `/assets/<name>` — a path no build file has. Any
  imported image in an SSR'd component had the same wrong `src`. The goja
  environment inherits Kit's top-level build config; override nothing there
  that decides a URL.
- Kit's `renderBuiltUrl` gives server consumers `base + filename`, and goja is
  a server consumer, so inheriting `_app/immutable/assets/[name].[hash][extname]`
  yields Kit's own server URL. Rolldown's asset hash is content-based, so it
  matches the file Kit's build copied into `client/`.
- Kit forbids reading a response header in a universal load during SSR unless
  `filterSerializedResponseHeaders` admits it (`load_response_header_not_serialized`).
  A test page that reads `content-type` fails for that reason, not skgo's.
- JSON files written with Bash under `example/web/src` were reformatted on disk
  (multi-line, wrapped) moments later. Write fixtures in already-formatted
  form or their stated bytes drift.
