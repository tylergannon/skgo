// Package buildinfo identifies the actual executing skgo, including project tools.
package buildinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
)

const Module = "github.com/tylergannon/skgo"

type Info struct {
	Executable  string           `json:"executable"`
	SkgoVersion string           `json:"skgoVersion"`
	Build       *debug.BuildInfo `json:"build"`
}

func Current() (Info, error) {
	b, ok := debug.ReadBuildInfo()
	if !ok {
		return Info{}, fmt.Errorf("skgo: executable has no recorded Go build information")
	}
	exe, err := os.Executable()
	if err != nil {
		return Info{}, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return Info{}, err
	}
	return Info{Executable: exe, SkgoVersion: Version(b), Build: b}, nil
}

func Version(b *debug.BuildInfo) string {
	if b.Main.Path == Module {
		return b.Main.Version
	}
	for _, d := range b.Deps {
		if d.Path == Module {
			return d.Version
		}
	}
	return "(devel)"
}
