package adapter

import (
	"fmt"
	"os"
	"testing"
)

var sharedInputsTemp string

func TestMain(m *testing.M) {
	if os.Getenv("SKGO_ADAPTER_PROCESS_FIXTURE") != "" {
		os.Exit(adapterProcessFixture())
	}
	var err error
	sharedInputsTemp, err = os.MkdirTemp("", "skgo-adapter-package-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	if err := os.RemoveAll(sharedInputsTemp); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}
