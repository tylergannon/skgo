# Issue 247: typed-load evolution fixture slice

On integrated main `d034f77`, the typed-load refresh test copies the entire
example. An unrelated demo consumer can therefore stop generation before this
test reaches its type-refresh contract. This slice replaces that copy with an
authored application under `internal/gen/testdata`, preserving the example
module and hooks type identities. Only dependency bootstrap comes from the
example; its route consumers and hooks do not.

Keep three generator attempts: initial named `OrderNumber` declarations and
methods; changing the matcher to `RevisedOrder` writes concrete fields and
accessors for both routes before stale `Label` handlers fail; repairing both
handlers allows generation and real HTTP requests to return independently
specified `Revised order #42` and `Revised order #7`, including selecting `b`.
The fixture owns the matcher and those two routes. Existing adjacent matcher,
prerender, dependency and shared-parameter tests remain outside this slice.

Capture one fresh narrow baseline and one narrow post-change run with elapsed
time and available CPU/RSS, accounting for all three generation attempts and
the child Go test. Independent validation owns bounded unrelated-consumer and
type-refresh fault checks. No browser/full-suite proof, runtime API changes,
Gimbal or additional implementation work belongs to this slice. The parent
handles the user-authorized commit and merge after independent validation.
