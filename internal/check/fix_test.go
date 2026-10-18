package check

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFixMissingToolsStillRunsOrdinaryChecks(t *testing.T) {
	root := t.TempDir()
	web := filepath.Join(root, "web", "src")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"go.mod":           "module example.com/missing\n\ngo 1.27.2\n",
		"app.go":           "package app\nvar Answer=42\n",
		"web/package.json": "{\"name\":\"missing\"}\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", t.TempDir())
	r := Run(context.Background(), Options{Root: root, Fix: true})
	if r.OK {
		t.Fatal("missing tools reported clean")
	}
	for _, name := range []string{"goimports-fix", "prettier-fix", "eslint-fix", "goimports", "prettier", "eslint", "svelte-check", "go-build", "go-vet", "staticcheck", "skgo-advice", "skgo"} {
		found := false
		for _, c := range r.Checks {
			if c.Name == name {
				found = true
				if c.Status == "complete" {
					t.Errorf("missing tool %s reported complete: %+v", name, c)
				}
			}
		}
		if !found {
			t.Errorf("missing tool prevented independent check %s", name)
		}
	}
	got, err := os.ReadFile(filepath.Join(root, "app.go"))
	if err != nil || string(got) != "package app\nvar Answer=42\n" {
		t.Fatalf("missing formatter changed source: %q %v", got, err)
	}
}

func TestFixFailureSurvivesSuccessfulOrdinaryTool(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "web", "node_modules", ".bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// A project tool that breaks only in write mode must not have its failed
	// fix invocation erased by a later read-only invocation that succeeds.
	tool := "#!/bin/sh\nif [ \"$2\" = '--fix' ]; then echo 'fixer failed'; exit 2; fi\necho 'All 1 file are correctly formatted'\necho 'Found no warnings or lint errors in 1 file'\n"
	if err := os.WriteFile(filepath.Join(bin, "vp"), []byte(tool), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{"go.mod": "module example.com/broken\n\ngo 1.27.2\n", "web/package.json": "{}\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := Run(context.Background(), Options{Root: root, Fix: true})
	if r.OK {
		t.Fatal("failed fixer reported clean")
	}
	seenFix, seenCheck := false, false
	for _, c := range r.Checks {
		if c.Name == "vite-plus-fix" {
			seenFix = c.Status == "failed" && c.Message == "fixer failed"
		}
		if c.Name == "vite-plus" {
			seenCheck = c.Status == "complete"
		}
	}
	if !seenFix || !seenCheck {
		t.Fatalf("fix failure or ordinary recheck lost: %+v", r.Checks)
	}
}
