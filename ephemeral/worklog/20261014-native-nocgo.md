# Native replacement without cgo

scope: User authorized stage1a only: remove cgo from the current native implementation, preserve the complete generated fixture, qualify native Mac ARM64/Linux AMD64, then stop. Reclamation and edit-class changes remain later work.

decision: Materialize the revision-locked loader in the caller-owned private host build view. Adapt Unix dlopen/dlsym and Darwin ARM64 JIT/cache calls through purego eafd1cca3a52; preserve upstream mmap functions and Linux MAP_32BIT. Keep loader/purego dependencies out of the SDK and CLI.

source: pkujhd/goloader2695cfc libdl/dl_unix.go and mmap/mmap_darwin_arm64.go, Apache-2.0. The research probe's adapters provided the starting point; its empty-string Dlopen call did not preserve dlopen(NULL) and requires explicit null-pointer ABI invocation.

correction: On native Mac, dlopen(NULL) can return the RTLD_DEFAULT sentinel. A test that demanded a different handle rejected correct behavior. Assert actual process-symbol lookup and independently known getpid instead; distinguish NULL from an empty C string by controlled mutation.
