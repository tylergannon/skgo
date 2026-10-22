# Simple tagged releases

implementation: Release responds to version-tag pushes and accepts an existing tag for publication retries. Signing and distribution use the immutable tagged source. Published archives are retained and missing uploads completed; the package step accepts its already-published tarball.

correction: Both npm packages use the tag's own version and compare the exact served integrity before reusing a published version. A Homebrew tap already on a newer version stays unchanged. Recovery is rerunning publication for the same tag, without a controller.
