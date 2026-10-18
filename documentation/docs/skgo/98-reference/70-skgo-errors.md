---
title: "skgo: errors and redirects"
summary: Errorf, HTTPError, Invalid, Issue, BadRequest and Redirect.
---

How a Go handler says no. `Errorf` returns an error with an HTTP status that
Kit renders through the nearest `+error.svelte`, the way `error()` does.
`Invalid` carries field-level validation messages back to a form or a remote
function's caller, with each message on the field it names, and `BadRequest`
is the 400 for input the handler could not decode. `NewRedirect` sends the
browser elsewhere, from a load, an action or a command. `CaughtError` is what
`HandleError` receives. Kit's guide is [Errors](https://svelte.dev/docs/kit/errors).

> PACKAGE: github.com/tylergannon/skgo
> SYMBOLS: HTTPError BadRequest ValidationIssue Invalid Issue Redirect CaughtError
