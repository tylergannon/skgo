package gen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Run the app's formatter once, after all frontend files have been generated.
func flushFrontendSources(cfg Config) error {
	if len(cfg.frontendFiles) == 0 {
		return nil
	}
	vp, err := frontendFormatter(cfg.Web)
	if err != nil || vp == "" {
		return err
	}
	paths := make([]string, 0, len(cfg.frontendFiles))
	for path := range cfg.frontendFiles {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	args := append([]string{"fmt", "--ignore-path", os.DevNull}, paths...)
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" && strings.HasSuffix(vp, ".cmd") {
		cmd = exec.Command("cmd.exe", append([]string{"/c", vp}, args...)...)
	} else {
		cmd = exec.Command(vp, args...)
	}
	cmd.Dir = cfg.Web
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("skgo: formatting generated frontend with vp fmt: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func frontendFormatter(web string) (string, error) {
	vp := filepath.Join(web, "node_modules", ".bin", "vp")
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(vp); err != nil {
			vp += ".cmd"
		}
	}
	if _, err := os.Stat(vp); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return vp, nil
}
