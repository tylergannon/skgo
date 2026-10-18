---
title: "skgo: remote functions"
summary: Query, Command, Form, LiveQuery and BatchQuery, with refresh and caller helpers.
---

The Go side of a `.remote.ts` module. A Go function in a `*.remote.go` file
marked with `Query`, `Command`, `Form`, `LiveQuery` or `BatchQuery` becomes an
export of the generated `.remote.ts` module next to it, with the argument and
result types Kit's client expects. Kit's guide is
[Remote functions](https://svelte.dev/docs/kit/remote-functions).

A command that changes data tells the client which queries to refresh with
`Refresh` and its relatives; `Requested` inspects which queries the caller
declared it depends on. `RequireArgument` and `RefuseArgument` are the
guards a query uses to say whether it takes an argument when the generated
signature cannot. The `Call`, `Kind` and `Remote` types are what the
generated registration hands the server; application code rarely touches
them.

> PACKAGE: github.com/tylergannon/skgo
> SYMBOLS: Query Command Form LiveQuery BatchQuery RequestedQuery Refresh RefreshNoArg RefreshRequested RefreshRequestedNoArg IgnoreRequested IgnoreRequestedNoArg ReconnectRequested ReconnectRequestedNoArg RequireArgument RefuseArgument RemoteCallerValues Marker Call Kind Remote Remotes NewRemote NewRemotes RemoteConfig RemoteSpec RemoteFunc RemoteInputsFunc BatchFunc LiveFunc
