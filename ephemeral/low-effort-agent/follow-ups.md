# Potential follow-up issues

- Offer an explicit, opt-in migration for existing applications to ignore `.skgo/` local build state. New scaffolds are fixed by #360; updating an authored `.gitignore` needs a deliberate migration surface. Sonnet raised this as nonblocking. Do not silently rewrite existing configuration.
- Correct cookie-write error messages that still imply events exist only in remote functions or that only commands may write cookies. The godoc fix in #363 describes loads, actions, forms and hooks accurately; the runtime diagnostics in `Event.mayWriteCookies` deserve the same accuracy. This is a separate, small behavior-text change.
