# Real Go–Zig binary consumer exchange

The next step authorized in the devalue conversation is a Zig client calling a
Go/skgo endpoint, exchanging binary views in both directions, and checking
contents and shared identity. The codec's existing 111-case exchange is already
done. skgo PR #287 supplies the production native query/command core and CLI;
this slice extends that core's admitted arguments with ArrayBuffer, all twelve
typed-array kinds and DataView, using the published devalue package.

A real loopback HTTP exchange must reach skgo's production remote dispatcher.
Go must inspect full backing bytes, view geometry, repeated versus distinct
view identity, shared buffers, distinct empty buffers and a graph cycle before
returning an independently constructed binary reply. Zig's actual response
path must decode that reply. Both query and command arguments must match pinned
Kit 3.0.0 and devalue 5.9.4 bytes. Native tests also inspect Zig graph content and
identity after response storage is released, with allocator failure coverage.

This is the generic Zig graph/CLI path. The generated Swift/JSON API, custom
transports, query caching, Map/Set canonicalization and Apple UI are separate
work under ephemeral/plans/zig-native-client.md. Keep the changes bounded to the
existing native consumer and its tests. Required delivery is passing repository
checks and independent review, a merged PR, synchronized main and task cleanup.
