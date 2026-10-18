---
title: skgo kit-patch
description: Configure or verify the Kit queue correction.
division: cli
section: Commands
order: 2060
---

SKGo corrects Kit's post-build prerender queue with a project-local pnpm patch. New applications are configured by `skgo new`; this command is for applications set up by hand and for CI to verify the installed bytes.

## Usage

`skgo kit-patch --help` prints:

```text
usage: go tool skgo kit-patch [--web web] [--workspace web] (--apply | --check)

--workspace, when supplied, must equal --web. Parent workspace mutation is refused.
--apply configures files only; run the frontend's VitePlus install, then use --check.
  -apply
    	pin the supported Kit release and configure the project-local pnpm patch (does not install)
  -check
    	verify configured files and actual installed Kit queue bytes
  -web string
    	selected frontend root containing package.json (default "web")
  -workspace string
    	assert the selected frontend is this root; parent workspaces are never modified
```
