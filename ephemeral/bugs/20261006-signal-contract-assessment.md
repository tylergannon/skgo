The test contract itself needs review before changing shutdown implementation.

`TestPrerenderInputsOuterVPCLISignalDrainsBlockedProducer` sends SIGTERM to `cmd.Process` (outer `vp`), waits for that command, and immediately calls `assertInputsReceiptDrained`. That assertion requires the Go process group and private directory to already be gone. It does not explicitly wait for the separate Vite owner to finish cleanup.

SKGo's current parent-loss behavior is asynchronous: `watchParent` checks parent identity on a 50ms interval, then awaits `session.fail`/cleanup. The outer launcher's exit is not itself an acknowledgement from that observer. `exec.Cmd.WaitDelay = 2s` plus inherited output pipes may incidentally delay Go's Wait; that is not an explicit cleanup-completion protocol.

The installed vite-plus 1.0.0 JS entry delegates build execution to its native binding. Reading this JS does not establish the native launcher's full signal-forwarding/reaping contract. Therefore this is a concrete synchronization concern in the test, NOT a proven explanation of the recorded intermittent failures, whose original diagnostics were lost.

Separate the promises under examination:

- An ordinary application/build failure handled inside the live adapter owner should finish owned cleanup before its build promise settles. #250 provides evidence of failure at this boundary.
- Gracefully terminating the actual Vite owner can test the cleanup handler owned by SKGo.
- Externally terminating an upstream launcher does not, without a demonstrated launcher guarantee, establish that a surviving child has already completed cleanup. If bounded eventual cleanup after parent loss is a supported requirement, test that event and its limit explicitly. If it is not a supported requirement, reconsider the test and the parent-watching complexity together rather than adding machinery merely to satisfy an unsupported assertion.

Do not fix this by blindly increasing sleeps or deleting leak assertions. First establish the upstream launcher contract and capture failure-stage/process-state evidence. The same reasoning does not automatically explain the Vite-owner signal failure.
