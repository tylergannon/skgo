finding: The first post-merge release qualification passed every generated-app and example browser scenario, including HMR, but npm returned E404 after the workflow had already created v0.3.0.

cause: npm trusted publishing is configured for .github/workflows/release.yml as a top-level workflow. Invoking that file as a reusable workflow from release-gate.yml changed the OIDC workflow identity, so the registry did not authorize the publish.

decision: release.yml owns the complete main-push sequence directly: qualify, derive the Conventional Commit version, publish through its trusted OIDC identity, then tag the exact SHA. The tag is now the final release action, so a publish failure cannot create another Go release tag.
