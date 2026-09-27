package check

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSvelteFormattingResultStatus(t *testing.T) {
	root := t.TempDir()
	web := filepath.Join(root, "web")
	page := filepath.Join(web, "src", "routes", "+page.svelte")
	if err := os.MkdirAll(filepath.Dir(page), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, []byte("<h1>Hello</h1>\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, output, status string
		exit                 int
	}{
		{name: "clean", status: "complete"},
		{name: "unformatted", output: "src/routes/+page.svelte", status: "failed", exit: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := filepath.Join(t.TempDir(), "prettier")
			script := "#!/bin/sh\n"
			if tc.output != "" {
				script += "printf '%s\\n' '" + tc.output + "'\n"
			}
			if tc.exit == 1 {
				script += "exit 1\n"
			}
			if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			check, diagnostics := checkSvelteFormatting(context.Background(), root, web, bin)
			if check.Status != tc.status {
				t.Fatalf("status=%q, want %q; message=%q diagnostics=%+v", check.Status, tc.status, check.Message, diagnostics)
			}
			if tc.exit == 0 {
				if len(diagnostics) != 0 {
					t.Fatalf("clean file produced diagnostics: %+v", diagnostics)
				}
				return
			}
			if len(diagnostics) != 1 || diagnostics[0].Severity != "error" || diagnostics[0].Location == nil || diagnostics[0].Location.File != "web/src/routes/+page.svelte" {
				t.Fatalf("formatting difference lost its authored location: %+v", diagnostics)
			}
		})
	}
}

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
