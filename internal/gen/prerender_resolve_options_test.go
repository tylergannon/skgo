package gen

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRealKitBuildAppliesRequestResolveOptions(t *testing.T) {
	t.Parallel()
	fixture := requireProductionFixture(t)
	read := func(path string) string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(fixture.app, "ui", "build", "prerendered", path))
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	body := read("options.html")
	for _, want := range []string{">Resolve options</h1>", "<p>inner-outer</p>", "post-load-state public-literal options fetched body headers-now-ready", `"x-public":"public-literal"`, `"x-late":"late-literal"`, `import(`} {
		if !strings.Contains(body, want) {
			t.Errorf("custom page lacks %q: %s", want, body)
		}
	}
	for _, denied := range []string{`"x-denied"`, `"x-default"`, `rel="modulepreload"`, "TRANSFORM_TOKEN", "INNER_TOKEN"} {
		if strings.Contains(body, denied) {
			t.Errorf("custom page retained %q", denied)
		}
	}
	if !regexp.MustCompile(`<link href="[^"]*/fixture\.[^"/]+\.woff2" rel="preload" as="font"`).MatchString(body) {
		t.Errorf("post-load font preload missing: %s", body)
	}
	if !strings.Contains(body, `rel="stylesheet"`) {
		t.Error("preload policy removed required stylesheet")
	}
	defaults := read("options-default.html")
	for _, want := range []string{">Default options</h1>", "options fetched body", `rel="modulepreload"`} {
		if !strings.Contains(defaults, want) {
			t.Errorf("default page lacks %q: %s", want, defaults)
		}
	}
	for _, denied := range []string{`"x-public"`, `"x-late"`, `"x-denied"`, `"x-default"`, `as="font"`} {
		if strings.Contains(defaults, denied) {
			t.Errorf("default page retained %q", denied)
		}
	}
	if !strings.Contains(fixture.buildOutput, "skgo prerender service stopped") {
		t.Error("build did not drain its service")
	}
}

func TestRealKitBuildSharesExplicitRendererDefaultsAndRejectsHeaderReads(t *testing.T) {
	t.Parallel()
	fixture := requireOptionsFixture(t)
	body, err := os.ReadFile(filepath.Join(fixture.app, "ui", "build", "prerendered", "options-default.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{">Default options</h1>", "options fetched body", `"x-default":"default-literal"`, `rel="modulepreload"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("explicit default page lacks %q: %s", want, body)
		}
	}
	for _, denied := range []string{`"x-public"`, `"x-late"`, `"x-denied"`, `as="font"`} {
		if strings.Contains(string(body), denied) {
			t.Errorf("explicit default page retained %q", denied)
		}
	}
	if !strings.Contains(fixture.buildOutput, "load_response_header_not_serialized") || !strings.Contains(fixture.buildOutput, "x-denied") {
		t.Errorf("Kit did not reject the universal-load header read: %s", fixture.buildOutput)
	}
	fixture.run(t, "TestSharedRendererDefaultFilter")
}
