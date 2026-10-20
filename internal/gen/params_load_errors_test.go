package gen

import (
	"strings"
	"testing"
)

const leafHint = "matcher/domain dependencies must be leaf packages"

// The hint names an import cycle through the matcher package, so it belongs
// only to an error that reports one. Every other compiler error stands alone.
func TestGoParamsLoadErrorsCarryHintOnlyForImportCycles(t *testing.T) {
	t.Parallel()
	const paramsSource = "package src\n\nimport \"example.com/app/domain\"\n\nvar Order = domain.Order\n"
	for _, tc := range []struct {
		name     string
		files    map[string]string
		want     string
		wantHint bool
	}{
		{
			name: "unused import in hooks.go",
			files: map[string]string{
				"web/src/params.go":      paramsSource,
				"web/src/hooks.go":       "package src\n\nimport \"example.com/app/businesslogic\"\n",
				"businesslogic/logic.go": "package businesslogic\n",
				"domain/domain.go":       "package domain\n\nconst Order = 1\n",
			},
			want: `"example.com/app/businesslogic" imported and not used`,
		},
		{
			name: "undefined name",
			files: map[string]string{
				"web/src/params.go": paramsSource,
				"web/src/hooks.go":  "package src\n\nvar _ = missing\n",
				"domain/domain.go":  "package domain\n\nconst Order = 1\n",
			},
			want: "undefined: missing",
		},
		{
			name: "domain imports the params package",
			files: map[string]string{
				"web/src/params.go": paramsSource,
				"domain/domain.go":  "package domain\n\nimport _ \"example.com/app/web/src\"\n\nconst Order = 1\n",
			},
			want:     "import cycle",
			wantHint: true,
		},
		{
			name: "domain imports a package that imports the params package",
			files: map[string]string{
				"web/src/params.go": paramsSource,
				"web/src/hooks.go":  "package src\n",
				"domain/domain.go":  "package domain\n\nimport _ \"example.com/app/caller\"\n\nconst Order = 1\n",
				"caller/caller.go":  "package caller\n\nimport _ \"example.com/app/web/src\"\n",
			},
			want:     "import cycle",
			wantHint: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := fakeApp(t)
			for path, source := range tc.files {
				writeSharedFixture(t, cfg.Web+"/..", path, source)
			}
			_, err := readGoParamMatchers(cfg)
			if err == nil {
				t.Fatal("readGoParamMatchers succeeded")
			}
			t.Logf("%v", err)
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error lacks %q: %v", tc.want, err)
			}
			if got := strings.Contains(err.Error(), leafHint); got != tc.wantHint {
				t.Fatalf("hint present = %v, want %v: %v", got, tc.wantHint, err)
			}
		})
	}
}
