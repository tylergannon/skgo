---
title: skgo/generate
summary: The application generator as a Go package, for build tools that embed it.
---

`skgo generate` as a library. A build tool that already loads the
application's packages can run the same generation in process, share its
`go/packages` bundle, and keep the persistent plan cache the command uses.
Most applications never import this package; the command and the
`go:generate` directive in the bindings package are enough.

> PACKAGE: github.com/tylergannon/skgo/generate
