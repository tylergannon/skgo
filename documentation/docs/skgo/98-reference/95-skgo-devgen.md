---
title: skgo/devgen
summary: Adapts an application-owned development generator to skgo dev.
---

For applications that generate code of their own during development.
`skgo dev --generator-config` names a tooling main that calls `Serve`; the
development server owns watching and sequencing, and asks the helper to
regenerate when its inputs change. Import it only from a tooling main, never
from the application.

> PACKAGE: github.com/tylergannon/skgo/devgen
