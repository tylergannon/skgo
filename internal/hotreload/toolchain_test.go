package hotreload

import (
	"strings"
	"testing"
)

func TestNativeCompilerFlags(t *testing.T) {
	for _, flag := range []string{"-race", "-gcflags=all=-N", "-tags=custom", "-trimpath"} {
		t.Run(flag, func(t *testing.T) {
			t.Setenv("GOFLAGS", flag)
			err := (Toolchain{}).checkFlags()
			if err == nil || !strings.Contains(err.Error(), "native development profile") {
				t.Fatalf("unsupported compiler profile accepted: %v", err)
			}
		})
	}
}
