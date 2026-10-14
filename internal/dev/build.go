package dev

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func (s *Server) buildFlags(ctx context.Context) ([]string, error) {
	flags := append([]string(nil), s.cfg.BuildFlags...)
	if s.cfg.Debug {
		return flags, nil
	}
	for _, flag := range flags {
		if flag == "-ldflags" || flag == "--ldflags" || strings.HasPrefix(flag, "-ldflags=") || strings.HasPrefix(flag, "--ldflags=") {
			return flags, nil
		}
	}
	// Preserve linker settings from both the invoking environment and go env -w.
	// A possible explicit setting only disables this optimization; Go still owns
	// parsing and validating the user's quoted GOFLAGS.
	cmd := exec.CommandContext(ctx, "go", "env", "GOFLAGS")
	cmd.Dir = s.cfg.Root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go env GOFLAGS failed: %w\n%s", err, output)
	}
	if strings.Contains(string(output), "-ldflags") {
		return flags, nil
	}
	return append([]string{"-ldflags=-w"}, flags...), nil
}
