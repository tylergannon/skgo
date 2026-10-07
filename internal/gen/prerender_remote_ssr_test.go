package gen

import "testing"

// The production bundle must reach the null-key artifact lookup without
// running the Go body or Kit's no-argument validator.
func testCompiledProductionNoArgumentNullReachesTheArtifactLookup(t *testing.T) {
	requireProductionFixture(t).run(t, "TestProductionNoArgumentNullUsesArtifactMiss")
}

func testCompiledProductionStoredPrerenderError(t *testing.T) {
	requireProductionFixture(t).run(t, "TestProductionStoredError")
}

func testCompiledProductionPrerenderTransformFailure(t *testing.T) {
	requireProductionFixture(t).run(t, "TestProductionTransformFailure")
}
