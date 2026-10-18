---
title: skgo/nativehost
summary: Runs the application's Go handler on a loopback origin for an embedded native shell.
---

The process an embedded native shell talks to. `nativehost` keeps the
application's Go handler on a retained loopback origin so the WebView loads
the same server a browser would, with the bundled Go or a hosted service
behind it.

> PACKAGE: github.com/tylergannon/skgo/nativehost
