package check

import (
	"path/filepath"
	"testing"
)

func TestVitePlusLintDiagnosticFromRealCLIShape(t *testing.T) {
	root := t.TempDir()
	web := filepath.Join(root, "web")
	output := []byte(`{"diagnostics":[{"message":"Type 'string' is not assignable to type 'number'.","code":"typescript(TS2322)","severity":"error","filename":"src/lib/probe.ts","labels":[{"span":{"offset":13,"length":7,"line":1,"column":14}}]}],"number_of_files":32}`)
	ds := parseVitePlusLint(root, web, output)
	if len(ds) != 1 || ds[0].Code != "typescript(TS2322)" || ds[0].Severity != "error" || ds[0].Location == nil || ds[0].Location.File != "web/src/lib/probe.ts" || ds[0].Location.Line != 1 || ds[0].Location.Column != 14 {
		t.Fatalf("Vite+ location was lost: %+v", ds)
	}
}

func TestVitePlusCompletionFromInstalledVersions(t *testing.T) {
	for _, output := range []string{
		"pass: All 20 files are correctly formatted\nFound 0 errors and 1 warning in 31 files",
		"pass: All 1 file are correctly formatted\npass: Found no warnings or lint errors in 1 file",
	} {
		if !vitePlusFormatted.MatchString(output) || !vitePlusLinted.MatchString(output) {
			t.Fatalf("clean Vite+ output was treated as incomplete: %s", output)
		}
	}
}
