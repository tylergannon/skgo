package gen

import "testing"

func TestEvolvedApplication(t *testing.T) {
	t.Parallel()
	t.Run("TestFormClientInputContractEvolution", testFormClientInputContractEvolution)
	t.Run("TestFormClientResultContractEvolution", testFormClientResultContractEvolution)
	t.Run("TestFormClientGenerationRecoversAndTracksContract", testFormClientGenerationRecoversAndTracksContract)
	t.Run("TestGenerateRecoversFromAStaleGeneratedFile", testGenerateRecoversFromAStaleGeneratedFile)
	t.Run("TestCheckAcceptsNullableWithGeneratorProjection", testCheckAcceptsNullableWithGeneratorProjection)
	t.Run("TestNamedDeferredPayloadSerializedNames", testNamedDeferredPayloadSerializedNames)
}

func TestProductionApplication(t *testing.T) {
	ready := startProductionBuild(builtProductionFixture)
	t.Parallel()
	if err := ready(); err != nil {
		t.Fatal(err)
	}
	t.Run("TestTypedLoadMatchersPrerenderNamedGoValues", testTypedLoadMatchersPrerenderNamedGoValues)
	t.Run("TestGoPagePrerenderRedirectBuildsKitsNativeArtifact", testGoPagePrerenderRedirectBuildsKitsNativeArtifact)
	t.Run("TestRealKitBuildSharesPrerenderLocalsAndWrapsTheRenderedResponse", testRealKitBuildSharesPrerenderLocalsAndWrapsTheRenderedResponse)
	t.Run("TestCompiledProductionPrerenderedEndpointsAndPages", testCompiledProductionPrerenderedEndpointsAndPages)
	t.Run("TestRealProductionStartsWithPrerenderedEndpoints", testRealProductionStartsWithPrerenderedEndpoints)
	t.Run("TestCompiledProductionNoArgumentNullReachesTheArtifactLookup", testCompiledProductionNoArgumentNullReachesTheArtifactLookup)
	t.Run("TestCompiledProductionStoredPrerenderError", testCompiledProductionStoredPrerenderError)
	t.Run("TestCompiledProductionPrerenderTransformFailure", testCompiledProductionPrerenderTransformFailure)
	t.Run("TestRealKitBuildAppliesRequestResolveOptions", testRealKitBuildAppliesRequestResolveOptions)

}

func TestProductionOptionsApplication(t *testing.T) {
	ready := startProductionBuild(builtOptionsFixture)
	t.Parallel()
	if err := ready(); err != nil {
		t.Fatal(err)
	}
	t.Run("TestRealKitBuildSharesExplicitRendererDefaultsAndRejectsHeaderReads", testRealKitBuildSharesExplicitRendererDefaultsAndRejectsHeaderReads)
}

func TestCheckApplication(t *testing.T) {
	t.Parallel()
	t.Run("TestRealCheckReportsWireAdviceAtAuthoredLocations", testRealCheckReportsWireAdviceAtAuthoredLocations)
	t.Run("TestReadOnlyCheckFindsCurrentWireFieldBeforeStaleLink", testReadOnlyCheckFindsCurrentWireFieldBeforeStaleLink)
}
