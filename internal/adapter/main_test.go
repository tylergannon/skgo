package adapter

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("SKGO_ADAPTER_PROCESS_FIXTURE") != "" {
		os.Exit(adapterProcessFixture())
	}
	os.Exit(m.Run())
}
