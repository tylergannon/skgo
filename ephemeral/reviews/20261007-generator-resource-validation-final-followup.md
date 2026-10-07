# Final independent parser follow-up

The relative-line-directive regression in the preceding follow-up is repaired.
The latest `packageSource` scans Go comment tokens, makes authored relative
directive filenames absolute against the original source directory, and adds
an initial directive that retains both line and column. Absolute directives and
empty-filename continuations remain untouched. Comment-token scanning avoids
changing directive-looking raw-string contents.

Independent revalidation passed:

```
go test -count=1 -v -run '^TestFreshPackages' ./internal/gen
go test -overlay=ephemeral/reviews/gen-resource-validation/probe.overlay.json -count=1 -v -run '^TestIndependentLaterLineDirective$' ./internal/gen
```

The permanent contracts cover aliased roots, active workspace and relative
replacement handling, nested embedded assets, literal raw-string preservation,
source location, zero standard-library parsing, parser/compiler errors, and
relative line/block directives. The independent original failing fixture now
reports the original module's `alternate.go:40:16` and passes. Evidence:
`gen-resource-validation/relative-directive-fixed.log` and
`independent-relative-directive-fixed.log`.

No remaining material parser or generator regression was found in this final
follow-up. The observation limits in
`20261007-generator-resource-validation-followup.md` still apply. This validator
did not rerun the full suite; the implementing agent owns its corrected normal
execution, toolchain/frontend prerequisites, process-tree measurement, and
delivery conclusion.
