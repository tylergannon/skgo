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

func TestCheckApplication(t *testing.T) {
	t.Parallel()
	t.Run("RealCheckReportsWireAdviceAtAuthoredLocations", testRealCheckReportsWireAdviceAtAuthoredLocations)
	t.Run("ReadOnlyCheckFindsCurrentWireFieldBeforeStaleLink", testReadOnlyCheckFindsCurrentWireFieldBeforeStaleLink)
}
