package skgo

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tylergannon/skgo/internal/envspec"
)

type environmentFixture struct {
	Site     string  `env:"SITE_NAME,public"`
	Release  string  `env:"RELEASE,public,static"`
	Enabled  bool    `env:"ENABLED,public"`
	Account  string  `env:"ACCOUNT,public"`
	Count    int     `env:"COUNT"`
	Ratio    float64 `env:"RATIO"`
	Secret   string  `env:"SECRET"`
	Optional *bool   `env:"OPTIONAL,public"`
}

func environmentBuild(t *testing.T) fstest.MapFS {
	t.Helper()
	schema := envspec.Schema{Fields: []envspec.Field{
		{Name: "SITE_NAME", Field: "Site", Type: "string", Public: true},
		{Name: "RELEASE", Field: "Release", Type: "string", Public: true, Static: true},
		{Name: "ENABLED", Field: "Enabled", Type: "bool", Public: true},
		{Name: "ACCOUNT", Field: "Account", Type: "string", Public: true},
		{Name: "COUNT", Field: "Count", Type: "int"},
		{Name: "RATIO", Field: "Ratio", Type: "float64"},
		{Name: "SECRET", Field: "Secret", Type: "string"},
		{Name: "OPTIONAL", Field: "Optional", Type: "bool", Public: true, Optional: true},
	}, Static: map[string]any{"RELEASE": "A"}}
	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	return fstest.MapFS{"env.json": {Data: data}}
}
func TestEnvironmentTypedStartupSnapshot(t *testing.T) {
	build := environmentBuild(t)
	for _, site := range []string{"Alpha", "Beta"} {
		for _, boolean := range []string{"true", "false", "yes", "no"} {
			raw := map[string]string{"SITE_NAME": site, "RELEASE": "B", "ENABLED": boolean, "ACCOUNT": "123", "COUNT": "123", "RATIO": "1.25", "SECRET": "private-sentinel"}
			calls := 0
			cfg, snapshot, err := LoadEnvironment[environmentFixture](build, func(name string) (string, bool) { calls++; v, ok := raw[name]; return v, ok })
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Site != site || cfg.Release != "A" || cfg.Account != "123" || cfg.Count != 123 || cfg.Ratio != 1.25 || cfg.Optional != nil || cfg.Enabled != (boolean == "true" || boolean == "yes") {
				t.Fatalf("incorrect typed snapshot: %#v", cfg)
			}
			before := calls
			raw["SITE_NAME"] = "Changed"
			public := snapshot.Public()
			public["SITE_NAME"] = "Mutated"
			if snapshot.Public()["SITE_NAME"] != site || calls != before {
				t.Fatal("snapshot changed or reread environment")
			}
			encoded, _ := json.Marshal(snapshot.Public())
			if strings.Contains(string(encoded), "private-sentinel") {
				t.Fatal("public projection leaked private value")
			}
			if _, ok := snapshot.DynamicPublic()["RELEASE"]; ok {
				t.Fatal("static value in dynamic public projection")
			}
		}
	}
}
func TestEnvironmentRejectsInvalidWithoutValues(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		unset       bool
	}{{"ENABLED", "sensitive-invalid", false}, {"COUNT", "9007199254740992", false}, {"COUNT", "1.2", false}, {"RATIO", "NaN", false}, {"RATIO", "Infinity", false}, {"ENABLED", "", false}, {"SITE_NAME", "", true}} {
		raw := map[string]string{"SITE_NAME": "Alpha", "ENABLED": "yes", "ACCOUNT": "123", "COUNT": "123", "RATIO": "1.25", "SECRET": "private-sentinel"}
		if tc.unset {
			delete(raw, tc.name)
		} else {
			raw[tc.name] = tc.value
		}
		_, _, err := LoadEnvironment[environmentFixture](environmentBuild(t), func(name string) (string, bool) { v, ok := raw[name]; return v, ok })
		if err == nil || !strings.Contains(err.Error(), tc.name) {
			t.Fatalf("%s: expected named failure, got %v", tc.name, err)
		}
		if tc.value != "" && strings.Contains(err.Error(), tc.value) {
			t.Fatalf("error leaked input: %v", err)
		}
	}
}
func TestEnvironmentEmptyStringAndOptionalPresence(t *testing.T) {
	raw := map[string]string{"SITE_NAME": "", "ENABLED": "No", "ACCOUNT": "false", "COUNT": "0", "RATIO": "0", "SECRET": "", "OPTIONAL": "YES"}
	cfg, _, err := LoadEnvironment[environmentFixture](environmentBuild(t), func(name string) (string, bool) { v, ok := raw[name]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site != "" || cfg.Account != "false" || cfg.Enabled || cfg.Optional == nil || !*cfg.Optional {
		t.Fatalf("wrong interpretation: %#v", cfg)
	}
}
