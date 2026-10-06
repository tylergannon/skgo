package gen

import "testing"

// The production bundle must reach the null-key artifact lookup without
// running the Go body or Kit's no-argument validator.
func TestCompiledProductionNoArgumentNullReachesTheArtifactLookup(t *testing.T) {
	t.Parallel()
	requireProductionFixture(t).run(t, "TestProductionNoArgumentNullUsesArtifactMiss")
}

func TestCompiledProductionStoredPrerenderError(t *testing.T) {
	t.Parallel()
	requireProductionFixture(t).run(t, "TestProductionStoredError")
}

func TestCompiledProductionPrerenderTransformFailure(t *testing.T) {
	t.Parallel()
	requireProductionFixture(t).run(t, "TestProductionTransformFailure")
}
