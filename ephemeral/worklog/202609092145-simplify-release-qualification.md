correction: PR #130's generated-project BDD was useful; the mistake was coupling it to the entire example suite and a repository-owned release-version subsystem. Preserve the generator work and simplify around it instead of rebuilding it.

decision: Release qualification runs the generated project's three scenarios in both modes plus the existing example HMR scenario tagged @release. The complete example suite remains unchanged and available outside the release path.

decision: Use pinned svu for Conventional Commit version selection. Do not maintain a project-specific parser and CLI for standard version semantics.
