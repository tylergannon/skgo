# Simple tagged releases

correction: The user requires merge -> independent qualification -> ordinary version tag -> release. Crash recovery is rerunning publication for that tag. Remove the reservation/controller machinery rather than extending it.

implementation: Release responds to version-tag pushes and accepts a tag for manual reruns. The qualified candidate is tagged by skgo-project after its existing agent/private-check qualification passes. Existing signing and distribution jobs use that immutable tag. Published archives are retained and missing uploads completed; the sv package step accepts its already-published tarball. The final reporting job records its own attempt identity and is not watched by a release controller.

configuration: skgo-project's publication environment uses a core-only write deploy key for tag pushes. Core's release environment now permits v* tags in addition to its existing main policy. No broader personal token or custom coordinator was introduced.

validation: The release YAML passed actionlint with exclusions for existing queue:max syntax and self-hosted runner labels. The actual npm retry script passed missing-version, identical-existing, mismatched-existing and registry-outage controls in the private qualification repository. Independent integrated review is in progress. No release has been published by this session yet.


review: Integrated independent review identified missing publisher-side qualification verification, latest-based package comparison on old-tag retries, and tap downgrades. Accepted all three. The source job verifies the exact passing assessment and approved evaluator before side effects. Both npm packages now always use the tag's own version and share exact-version publication/reuse, removing the latest-comparison optimization. A tap already on a newer version is left alone. No recovery controller was added.

proof: Actual source-job shell controls accept the exact qualified source and reject failed/unapproved assessments. The shared npm block passed missing/existing/mismatched/outage controls and refuses a latest lookup. Actual adapter packing through vp was byte-identical across two packs; vp's publish dry run accepted it without publication. The real tap command behind the new workflow guard preserved 0.43.0 while recovering v0.42.0 and advanced 0.41.0 to 0.42.0. Actions lint passes. These are focused workflow controls; live automatic publication remains unproved until merge.
