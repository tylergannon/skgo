# SKGo Zig remote client

The Zig core prepares Kit query and command requests and decodes their response
envelopes. The `skgo-remote` CLI executes those requests over HTTP:

```text
skgo-remote ORIGIN BASE ID query|command DEVALUE
```

Arguments admit primitives, plain/null-prototype objects, arrays, ArrayBuffer,
all twelve typed-array kinds and DataView. Query and command payloads match Kit
3.0.0 with devalue 5.9.4. Binary values retain backing bytes, shared buffers,
view identity and cycles through Go remote handlers. A subview transmits the
whole backing buffer; use the codec's `uint8ArrayCopy` to copy visible bytes.
Swift and C# use a finite JSON model boundary; each exported SDK describes its
language wrapper separately. The protocol/cache and C ABI here are shared.

From this directory, `mise exec -- zig build test install` builds the core and
CLI. From the native-client module root,
`go test -tags skgo_qualification -count=1 -race -v ./sdk` exercises actual
query and command HTTP calls against the production remote dispatcher.
