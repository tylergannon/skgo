---
title: skgo/templateapi
summary: The development-time interface a plugin implements to add templates and add-ons.
---

What a plugin exports. A native Go plugin exports `SKGoPluginV1() Plugin`
and is built for the exact `skgo` host that invokes it; `skgo plugin install`
builds it and `skgo new --template` and `skgo add` run it. Plugins are
trusted local code and are never loaded by a generated application.

> PACKAGE: github.com/tylergannon/skgo/templateapi
