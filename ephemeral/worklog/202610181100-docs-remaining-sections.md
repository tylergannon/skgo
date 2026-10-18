# Remaining documentation sections, one pass

correction: the user rejected one-PR-per-section for docs. A docs change needs
the renderer check (under a second) and the site build; splitting it across
PRs bought nothing because `docs:` commits did not release. All remaining
sections (Build and deploy, Advanced, Best practices, Appendix, CLI add-ons
and FAQ, the AI division) land in this one PR, merged without waiting on CI.

decision: `docs:` commits now cut a patch release (internal/releaseversion).
The documentation ships in the module and the site serves the pinned tag, so
a docs change that does not release never reaches a reader.

friction: the draft pages from skgo-project PR #157 were the harvest source;
each writer verified claims against source and dropped what it could not.
