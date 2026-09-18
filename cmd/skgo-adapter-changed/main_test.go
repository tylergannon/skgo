package main

import (
	"os"
	"strings"
	"testing"
)

func TestAutomaticSVReleaseRequiresManualBootstrap(t *testing.T) {
	err := unpublishedError("@skgo/sv", true)
	if err == nil {
		t.Fatal("automatic release accepted an unpublished package")
	}
	for _, want := range []string{"automatic release cannot perform its first publication", "internal/sv/README.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if err := unpublishedError("@skgo/sv", false); err != nil {
		t.Fatalf("manual changed check rejected unpublished package: %v", err)
	}
}

func TestSVReleaseWorkflowRefusesAutomaticBootstrap(t *testing.T) {
	workflow, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	want := "-dir internal/sv -package @skgo/sv -require-published"
	if !strings.Contains(text, want) {
		t.Fatalf("sv-npm changed check does not contain %q", want)
	}
	if count := strings.Count(text, "-require-published"); count != 1 {
		t.Fatalf("release workflow contains -require-published %d times, want only the sv-npm changed check", count)
	}
}
