---
title: llms.txt
summary: This documentation as plain markdown for language models, following the llms.txt convention.
---

The SKGo site follows the [llms.txt](https://llmstxt.org/) convention for making documentation available to large language models and the applications that use them. There is one root file:

- [`/llms.txt`](/llms.txt): what SKGo is, and where each division's markdown is

...and one file per division:

- `/docs/skgo/llms.txt`: every SKGo page
- `/docs/cli/llms.txt`: every CLI page
- `/docs/ai/llms.txt`: every AI page

Each page is also available alone at `/docs/<division>/<slug>/llms.txt`; `/docs/ai/mcp/llms.txt` is the [MCP overview](mcp).

## What the files contain

Every file is markdown served as `text/plain; charset=utf-8`. A page's file is its title as a heading followed by its markdown, and carries a `Link` header naming the HTML page as canonical. A division's file opens with a `<SYSTEM>` line naming the division, then holds every page in navigation order.

There is no abridged version and no file that combines the three divisions. The files describe the SKGo release the site is built with.
