package remotearg

import (
	"testing"
	"time"
)

func mustTime(t *testing.T, iso string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, iso)
	if err != nil {
		t.Fatalf("parsing %q: %v", iso, err)
	}
	return parsed
}
