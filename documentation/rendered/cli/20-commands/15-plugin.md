---
title: "skgo plugin"
summary: "Install or remove plugin source"
---

Plugins contribute templates to `skgo new` and add-ons to `skgo add`. They are built from source for this exact `skgo` host, which is why installing one needs Git, Go and cgo.

```sh
skgo plugin install MODULE[@QUERY] | remove MODULE
```

Installs trusted Git source for this executable, which contributes templates
and add-ons to `skgo new` and `skgo add`. Requires Git, Go and cgo.
Source generation and plugin initialization execute code. Removal affects only
this host's managed selection.
