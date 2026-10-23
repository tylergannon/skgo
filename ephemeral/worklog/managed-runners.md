# Managed runner migration

- decision: Exercise the existing plugin contracts and cross-platform compiler/wire witnesses on disposable managed workers before moving the rest of the workload. Keep fork PR jobs on GitHub-hosted workers.
- correction: Delivery means working replacement plus weekly maintenance, not an open-ended hardening phase.
- evidence: The first managed Linux CLI plugin run passed; native Go contracts passed on a second disposable worker. Both containers were removed.
- friction: CLT Swift 6.4's default build engine omitted its shipped Testing macro plugin. Switching to the deprecated native engine also lost Testing module discovery. Explicitly loading the shipped macro library made a standard Swift Testing probe pass; use that toolchain-local discovery for the SDK test steps and rerun the actual workload.
- decision: Release workflow supports an explicit build-only dispatch, allowing real signed packages to qualify replacement runners without republishing npm or GitHub assets.
