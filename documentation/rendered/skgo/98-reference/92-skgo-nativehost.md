---
title: "skgo/nativehost"
summary: "Runs the application's Go handler on a loopback origin for an embedded native shell."
---

The process an embedded native shell talks to. `nativehost` keeps the
application's Go handler on a retained loopback origin so the WebView loads
the same server a browser would, with the bundled Go or a hosted service
behind it.

## Host

```go
type Host struct {
	// contains filtered or unexported fields
}
```

### Recover

```go
func (h *Host) Recover() error
```

Recover rebinds the same origin without recreating the application's state. A failed rebind retains the handler and origin for another recovery attempt.

### Start

```go
func (h *Host) Start() (string, error)
```

### Stop

```go
func (h *Host) Stop() error
```

## New

```go
func New(create func(string) (http.Handler, error)) *Host
```
