---
title: "skgo/devgen"
summary: "Adapts an application-owned development generator to skgo dev."
---

For applications that generate code of their own during development.
`skgo dev --generator-config` names a tooling main that calls `Serve`; the
development server owns watching and sequencing, and asks the helper to
regenerate when its inputs change. Import it only from a tooling main, never
from the application.

## Request

```go
type Request struct {
	ID       uint64   `json:"id"`
	Epoch    uint64   `json:"epoch"`
	Revision uint64   `json:"revision"`
	Full     bool     `json:"full"`
	Paths    []string `json:"paths"`
}
```

Request selects one serialized generation pass. Paths are cumulative canonical absolute observed changes, including removed paths. Full requests require the generator to reconcile all its producers and owned outputs. Epoch is separate from Revision: non-Go inputs need not change SKGO's captured compiler inputs.

## Response

```go
type Response struct {
	ID        uint64 `json:"id"`
	Generated bool   `json:"generated"`
	Error     string `json:"error,omitempty"`
}
```

Response is the wire result. Generated means application-consumed owned output changed, was removed, or was repaired, including changes made before an error.

## Serve

```go
func Serve(ctx context.Context, callback func(context.Context, Request) (bool, error)) error
```

Serve runs callback serially until stdin closes or the protocol fails. Return changedSoFar on partial publication errors. Equal-byte writes, private cache updates, and analysis alone must return false. Callback state survives ordinary application replacement; helper source changes create a fresh process.

## ServeIO

```go
func ServeIO(ctx context.Context, in io.Reader, out io.Writer, callback func(context.Context, Request) (bool, error)) error
```

ServeIO is Serve with explicit streams, useful for tooling transport tests.
