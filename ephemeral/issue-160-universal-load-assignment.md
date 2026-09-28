# Issue #160: pure universal loads

Implement the invoice-screen example discussed in
https://github.com/tylergannon/skgo/issues/160. A Go server load supplies invoice
data; an ordinary authored `+page.ts` universal load derives the displayed rows
and total. The same transformation must execute inside goja for the initial
document and through Kit's own client during hydration and navigation.

Use an invoice dataset supplied by Go, including a fixed reference date, due
dates, and outstanding balances. The universal load marks overdue invoices,
filters on `?overdue=1`, sorts by due date, and totals the displayed balances.
The fixture should contain an overdue unpaid invoice, a future unpaid invoice,
and a paid invoice, so the filter cannot pass accidentally. Go supplies all rows
the visitor is authorized to see; the browser filter is presentation logic.

## Scope and constraints

Own the runtime/adapter changes needed for this capability and its example and
tests. Follow the repository's AGENTS.md and preserve other work in the checkout.
Kit is the specification: independently map the relevant behavior from
`/Users/tyler/src/skgo/ephemeral/inspiration/reference/kit@3.0.0-next.28`, with
`ephemeral/sveltekit-current/SKILL.md` as the local guide. Preserve Kit's rules
for server data, universal results, hydration, and dependency-driven reruns.

This assignment covers pure universal loads consuming Go data and URL inputs.
Application I/O stays in Go. Universal `fetch`, `setHeaders`, and full loading
API parity are outside this assignment; they are possible later work, not
declared permanent exclusions. Report any additional compatibility boundary
actually encountered. Keep the implementation within the existing architecture.

## Definition of done

- A Go test against the real application handler proves that a direct visit
  already contains the expected filtered invoice rows and total in its HTML.
  Expectations come from the supplied fixture.
- A browser scenario proves that those values survive hydration and that
  switching the URL filter through Kit navigation produces the expected full
  list and total without replacing the document. With server loads independent
  of the filter parameter, that change reuses their data without another server
  load request.
- The behavior is demonstrated in production and dev, with the repository's
  required checks and independent validation completed. Inspect the running
  example as a visitor. Report what was demonstrated and any remaining boundary;
  a successful build alone does not establish compatibility.

Deliver the working change and its tests. Choose the implementation approach
from the pinned source and current code; this assignment prescribes the outcome.
