package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestVersionIdentifiesAdoptedSKGo(t *testing.T) {
	for _, tc := range []struct {
		name string
		info *debug.BuildInfo
		want string
	}{
		{"unavailable", nil, "(devel)"},
		{"released CLI", &debug.BuildInfo{Main: debug.Module{Path: Module, Version: "v0.41.2"}}, "v0.41.2"},
		{"dirty VCS checkout", &debug.BuildInfo{Main: debug.Module{Path: Module, Version: "v0.37.6-0.20261018185716-ed0e816b562c+dirty"}, Settings: []debug.BuildSetting{{Key: "vcs", Value: "git"}, {Key: "vcs.modified", Value: "true"}}}, "(devel)"},
		{"clean tagged checkout", &debug.BuildInfo{Main: debug.Module{Path: Module, Version: "v0.41.2"}, Settings: []debug.BuildSetting{{Key: "vcs", Value: "git"}, {Key: "vcs.modified", Value: "false"}}}, "(devel)"},
		{"checkout CLI", &debug.BuildInfo{Main: debug.Module{Path: Module, Version: "(devel)"}}, "(devel)"},
		{"consumer", &debug.BuildInfo{Main: debug.Module{Path: "example.com/app", Version: "v9.8.7"}, Deps: []*debug.Module{{Path: Module, Version: "v0.41.2"}}}, "v0.41.2"},
		{"local replacement", &debug.BuildInfo{Deps: []*debug.Module{{Path: Module, Version: "v0.41.2", Replace: &debug.Module{Path: "../skgo"}}}}, "(devel)"},
		{"versioned replacement", &debug.BuildInfo{Deps: []*debug.Module{{Path: Module, Version: "v0.41.2", Replace: &debug.Module{Path: "example.com/fork", Version: "v1.2.3"}}}}, "(devel)"},
		{"workspace", &debug.BuildInfo{Deps: []*debug.Module{{Path: Module}}}, "(devel)"},
		{"unrelated app", &debug.BuildInfo{Main: debug.Module{Path: "example.com/app", Version: "v9.8.7"}}, "(devel)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Version(tc.info); got != tc.want {
				t.Fatalf("Version=%q, want %q", got, tc.want)
			}
		})
	}
}
