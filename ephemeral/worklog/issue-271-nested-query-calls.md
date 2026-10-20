# Issue 271: queries called from Go

- Decided 2026-10-19 (Tyler): a query's Go function called from Go is an
  ordinary call. No `skgo.Call`-style wrapper; `skgo.Refresh` is the way to
  send a value to the page. Documented in remote-functions.md, cookies.md and
  differences.md.
- Kit 3.1.0 fact the issue missed: a query a command awaits at top level is not
  only cookie-restricted but deduplicated per request and serialized into the
  command response (`get_response` registers it in `state.remote.implicit`;
  `collect_remote_data` emits `data.q`; the client applies it and treats it as
  handling a requested key). Undocumented in Kit's docs. Any future
  server-side query-call API must mirror all three, not the cookie refusal
  alone.
- A generated command's `ctx` is `event.Context()`, which already hides caller
  URL/params; only cookie mutability is shared with direct query calls.
