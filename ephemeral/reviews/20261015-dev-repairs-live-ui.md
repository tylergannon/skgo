# Live UI evidence: dev repairs (2026-10-15)

Source-blind browser observations only; no project source read. Successful screenshots were inspected and not retained. The CUA surface used here exposed no console-log reader.

- Production `http://127.0.0.1:8088/todos` rendered four todos: `write the adapter`, `serve remote functions`, `the left of the pair`, and `the right of the pair`. Opened the first todo, renamed it to `UI validation check`, then restored `write the adapter`; both values appeared in the UI.
- Production `/items/42` rendered `Item 42` and `Widget 42`. Previewing `UI smoke check` displayed `id:string:42` and the preview text. Forms and navigation rendered; styling was bare/default and the top navigation overflowed horizontally.
- First native `:8087/todos` navigation occurred before Go UDS readiness and visibly returned `500 Internal Error`; no behavior conclusion drawn from that attempt.
- Once native readiness was announced, `:8087/todos` rendered the four todos. Filled the Add field with `review-draft-913` without submitting. After a title edit, without reload, heading updated to `Todos review HMR913`, but the draft was lost and the field returned to empty `new todo`.
- Opened native `/items/42`; visible value was `Widget 42`. No later Go edit was observed because checks were stopped at the user's request to ship.
