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
The Swift JSON boundary has its own finite model profile, described in
[`../swift/README.md`](../swift/README.md).

From this directory, `mise exec -- zig build test install` builds the core and
CLI. From the SKGo root, `go test -count=1 -race -v ./native` exercises actual
query and command HTTP calls against the production remote dispatcher.
