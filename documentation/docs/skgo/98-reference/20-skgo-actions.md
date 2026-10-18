---
title: "skgo: actions"
summary: "Form actions in Go: Action, DefaultAction, Fail and the generated submit path."
---

The Go side of a `+page.server.ts` `actions` export. Each action is a Go
function marked with `Action` or one of its variants; a browser with
JavaScript off posts the form and gets a full page back, a browser with
JavaScript on gets the result without a reload, exactly as in Kit. Kit's guide
is [Form actions](form-actions).

`Fail` returns validation data with a 4xx status and keeps the page rendered;
a `*Redirect` from the [errors and redirects](skgo-errors) page sends the
browser elsewhere after a successful submit. `SubmitForm` and `FormClient` are
the server-side client the generated Swift and Go callers use to post an
action the way a browser would.

> PACKAGE: github.com/tylergannon/skgo
> SYMBOLS: Action DefaultAction ActionWithFailure ActionNoData Fail SubmitForm FormClient DecodeForm Actions ActionSpec PageAction NewActions NewPageAction
