package gen

import "testing"

func testPrerenderFetchReportsUnavailableAppDestinations(t *testing.T) {
	requireProductionFixture(t).run(t, "TestPrerenderFetchDestinationsMatchServedCapabilities")
}
