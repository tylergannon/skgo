---
title: skgo add
description: Apply an add-on to an application.
division: cli
section: Commands
order: 2020
---

An add-on is a reusable change to an existing application. `tailwindcss` is built in: it adds Tailwind CSS and keeps the application's Go source out of Tailwind's scanning. Others come from installed plugins. `skgo add --help` lists what this binary knows.

## Usage

```text skgo add -h
```

Install a plugin before selecting one of its add-ons. Installing a plugin alone does not change the application.
