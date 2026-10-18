---
title: skgo plugin
description: Install or remove plugin source.
division: cli
section: Commands
order: 2015
---

Plugins contribute templates to `skgo new` and add-ons to `skgo add`. They are built from source for this exact `skgo` host, which is why installing one needs Git, Go and cgo.

## Usage

`skgo plugin --help` prints:

```text
usage: skgo plugin install MODULE[@QUERY]
       skgo plugin remove MODULE

Install trusted Git source for this executable. Requires Git, Go and cgo. Source generation and plugin initialization execute code.
Removal affects only this host's managed selection.
```
