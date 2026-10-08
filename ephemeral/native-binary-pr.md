The native Zig client rejected ArrayBuffer, typed arrays and DataView before it could call Go. Admit those nodes in the existing query/command core, matching Kit 3.0.0's delegation to devalue, and pin the Zig dependency to the immutable devalue v5.0.0 source archive and fetched package hash.

Real loopback HTTP tests now send all twelve typed-array kinds and DataView through skgo's production remote dispatcher. Go inspects literal backing bytes, geometry, repeated and distinct views, shared and distinct empty buffers, and a cycle, then returns a fresh graph with different bytes. Zig decodes that reply and emits the exact upstream-recorded document. Binary support is available through the generic Zig graph API and CLI; the Swift JSON boundary retains its finite model contract.

Validation at `76a6ebd603f297c684e0f3ff55a09024cf7ee420`:
- [Actual query/command HTTP calls and existing Swift boundary tests](https://github.com/tylergannon/skgo/blob/76a6ebd603f297c684e0f3ff55a09024cf7ee420/ephemeral/native-binary-focused.txt): `go test -count=1 -race -v ./native` passed.
- [Root and fresh-built example tests](https://github.com/tylergannon/skgo/blob/76a6ebd603f297c684e0f3ff55a09024cf7ee420/ephemeral/native-binary-final-tests.txt): `just test` passed; `just vet` passed.
- Native Zig Debug and ReleaseSafe tests passed, including direct decoded-content/identity checks after releasing response bytes and exhaustive allocation-failure cleanup. Expected query/command/reply bytes were recorded from installed Kit 3.0.0 and its resolved devalue 5.9.4 with independently constructed JavaScript graphs.

Independent review is in progress; this draft is not yet merge-ready.
