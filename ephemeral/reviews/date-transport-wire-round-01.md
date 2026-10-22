# Date transport wire declaration review

## Assessment

The core change is correct for the reported defect and consistent with the codec. `transportEntryPayload` asks polytype's `Lower` for the same grammar the codec generator uses (`codecs.go`, same `grammar.Root{Type, Position}` shape), follows `Ref` chains, and returns `string` only when the leaf is `*typegrammar.Time`. Every other transported struct still takes the unchanged `transportPayload` path, so existing object payloads are unaffected. The `Ref` loop cannot cycle for Go defined types. I found no verifiable bug in the local core change.

## Local checks

The focused transport tests and the full generator package passed. Restoring the earlier declaration logic caused the Date and LocalDate regression assertions to fail. Go vet was clean.

## Follow-up

The transported-time declaration and codec payload must continue to agree. Documentation should describe time payloads as strings and avoid claiming that every transported value has an object payload.
