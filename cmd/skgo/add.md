---
title: skgo add
description: Apply an add-on to an application.
division: cli
section: Commands
order: 2020
---

An add-on is a reusable change to an existing application. `tailwindcss` and `ai-tools` are built in: `tailwindcss` adds Tailwind CSS and keeps the application's Go source out of Tailwind's scanning, and [`ai-tools`](/docs/ai/ai-tools) sets up coding agents at the application root. Others come from installed plugins. `skgo add --help` lists what this binary knows.

## Usage

```text skgo add -h
```

Install a plugin before selecting one of its add-ons. Installing a plugin alone does not change the application.
