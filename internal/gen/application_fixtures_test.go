package gen

import "testing"

func TestEvolvedApplication(t *testing.T) {
	t.Parallel()
	t.Run("FormClientInputContractEvolution", testFormClientInputContractEvolution)
	t.Run("FormClientResultContractEvolution", testFormClientResultContractEvolution)
	t.Run("FormClientGenerationRecoversAndTracksContract", testFormClientGenerationRecoversAndTracksContract)
	t.Run("GenerateRecoversFromAStaleGeneratedFile", testGenerateRecoversFromAStaleGeneratedFile)
	t.Run("CheckAcceptsNullableWithGeneratorProjection", testCheckAcceptsNullableWithGeneratorProjection)
	t.Run("NamedDeferredPayloadSerializedNames", testNamedDeferredPayloadSerializedNames)
}

func TestProductionApplication(t *testing.T) {
	ready := startProductionBuild(builtProductionFixture)
	t.Parallel()
	if err := ready(); err != nil {
		t.Fatal(err)
	}
	t.Run("TypedLoadMatchersPrerenderNamedGoValues", testTypedLoadMatchersPrerenderNamedGoValues)
	t.Run("GoPagePrerenderRedirectBuildsKitsNativeArtifact", testGoPagePrerenderRedirectBuildsKitsNativeArtifact)
	t.Run("RealKitBuildSharesPrerenderLocalsAndWrapsTheRenderedResponse", testRealKitBuildSharesPrerenderLocalsAndWrapsTheRenderedResponse)
	t.Run("CompiledProductionPrerenderedEndpointsAndPages", testCompiledProductionPrerenderedEndpointsAndPages)
	t.Run("RealProductionStartsWithPrerenderedEndpoints", testRealProductionStartsWithPrerenderedEndpoints)
	t.Run("CompiledProductionNoArgumentNullReachesTheArtifactLookup", testCompiledProductionNoArgumentNullReachesTheArtifactLookup)
	t.Run("CompiledProductionStoredPrerenderError", testCompiledProductionStoredPrerenderError)
	t.Run("CompiledProductionPrerenderTransformFailure", testCompiledProductionPrerenderTransformFailure)
	t.Run("RealKitBuildAppliesRequestResolveOptions", testRealKitBuildAppliesRequestResolveOptions)

}

func TestProductionOptionsApplication(t *testing.T) {
	ready := startProductionBuild(builtOptionsFixture)
	t.Parallel()
	if err := ready(); err != nil {
		t.Fatal(err)
	}
	t.Run("RealKitBuildSharesExplicitRendererDefaultsAndRejectsHeaderReads", testRealKitBuildSharesExplicitRendererDefaultsAndRejectsHeaderReads)
}

func TestCheckApplication(t *testing.T) {
	t.Parallel()
	t.Run("RealCheckReportsWireAdviceAtAuthoredLocations", testRealCheckReportsWireAdviceAtAuthoredLocations)
	t.Run("ReadOnlyCheckFindsCurrentWireFieldBeforeStaleLink", testReadOnlyCheckFindsCurrentWireFieldBeforeStaleLink)
}
