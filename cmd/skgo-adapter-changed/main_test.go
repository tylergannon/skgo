package main

import (
	"os"
	"path/filepath"
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

func TestStampVersionPreservesNestedVersionsAndFormatting(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "package.json")
	original := `{ "devEngines": {"packageManager": {"version": "12.9.1"}},
  "version" : "0.0.0-dev", "nested": [{"version":"9"}] }
`
	want := `{ "devEngines": {"packageManager": {"version": "12.9.1"}},
  "version" : "0.31.0", "nested": [{"version":"9"}] }
`
	if err := os.WriteFile(file, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := stampVersion(dir, "0.31.0"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("stamped package = %s; want %s", got, want)
	}
}

func TestStampVersionRejectsInvalidPackageWithoutWriting(t *testing.T) {
	for _, input := range []string{
		`{"nested":{"version":"1"}}`,
		`{"version":"1","version":"2"}`,
		`{"version":null}`,
		`{"version":1}`,
		`[{"version":"1"}]`,
		`{"version":"1"} trailing`,
	} {
		t.Run(input, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "package.json")
			if err := os.WriteFile(file, []byte(input), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := stampVersion(dir, "0.31.0"); err == nil {
				t.Fatal("accepted invalid package")
			}
			got, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != input {
				t.Fatalf("changed rejected package to %s", got)
			}
		})
	}
}
