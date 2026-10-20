package gen

import (
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Run the app's formatter once, after all frontend files have been generated.
func flushFrontendSources(cfg Config) error {
	return formatFrontendFiles(cfg, cfg.Web)
}

// formatGeneratedFrontend formats generated sources, keyed by their paths under
// web, as temporary copies in one invocation; formatter failure cannot leave
// half-written generated project files.
func formatGeneratedFrontend(web string, files map[string]string) (map[string]string, error) {
	dir, err := os.MkdirTemp("", "skgo-format-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"skgo-generated-output","private":true}`), 0600); err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(web, "node_modules")); err == nil {
		if err := os.Symlink(filepath.Join(web, "node_modules"), filepath.Join(dir, "node_modules")); err != nil {
			return nil, err
		}
	}
	stage := Config{Web: web, frontendFiles: map[string]struct{}{}}
	originals := map[string]string{}
	for path, source := range files {
		relative, err := filepath.Rel(web, path)
		if err != nil {
			return nil, err
		}
		target := filepath.Join(dir, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(target, []byte(source), 0644); err != nil {
			return nil, err
		}
		stage.frontendFiles[target] = struct{}{}
		originals[target] = path
	}
	if err := formatFrontendFiles(stage, dir); err != nil {
		return nil, err
	}
	formatted := make(map[string]string, len(files))
	for target, path := range originals {
		data, err := os.ReadFile(target)
		if err != nil {
			return nil, err
		}
		formatted[path] = string(data)
	}
	return formatted, nil
}

// frontendFormat formats a retained plan's frontend sources while its Go
// compiles. The private compiler tree keeps the unformatted text; only the
// authored tree, which Kit reads, needs the formatter's output.
type frontendFormat struct {
	done  chan struct{}
	files map[string]string
	err   error
}

func startFrontendFormat(web string, view *generationSourceView, files map[string]string) *frontendFormat {
	f := &frontendFormat{done: make(chan struct{})}
	go func() {
		defer close(f.done)
		formatted, err := formatGeneratedFrontend(web, files)
		if err != nil {
			f.err = fmt.Errorf("%s", view.originalText(err.Error()))
			return
		}
		f.files = make(map[string]string, len(formatted))
		for path, source := range formatted {
			f.files[view.mapPath(path, false)] = view.originalText(source)
		}
	}()
	return f
}

// formatted returns the plan as it is published, waiting for its formatter.
func (plan generationPlan) formatted() (generationPlan, error) {
	if plan.frontend == nil {
		return plan, nil
	}
	<-plan.frontend.done
	if plan.frontend.err != nil {
		return generationPlan{}, plan.frontend.err
	}
	files := maps.Clone(plan.Files)
	maps.Copy(files, plan.frontend.files)
	return generationPlan{Files: files, GoFiles: plan.GoFiles}, nil
}

func formatFrontendFiles(cfg Config, dir string) error {
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
	cmd.Dir = dir
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
