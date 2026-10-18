package check

import (
	"context"
	"os/exec"
	"strings"
)

// fixProject uses the same tools and selection as checking. No compiler,
// analyzer suggestion, generator, or dependency installation is a safe fix.
// All results are retained, and Run subsequently checks the resulting project.
func fixProject(ctx context.Context, root, web, out string) []Check {
	var checks []Check
	run := func(name, dir, bin string, args ...string) {
		c := Check{Name: name + "-fix", Status: "complete"}
		if bin == "" {
			c.Status = "failed"
			c.Message = name + " is missing"
		} else if output, err := command(ctx, dir, nil, bin, args...); err != nil {
			c.Status = "failed"
			c.Message = strings.TrimSpace(string(output))
			if c.Message == "" {
				c.Message = err.Error()
			}
		}
		checks = append(checks, c)
	}
	files, err := goImportFiles(root, out)
	if err != nil {
		checks = append(checks, Check{Name: "goimports-fix", Status: "failed", Message: err.Error()})
	} else {
		bin, _ := exec.LookPath("goimports")
		if len(files) > 0 || bin == "" {
			run("goimports", root, bin, append([]string{"-w"}, files...)...)
		}
	}
	if bin := localBin(web, "vp"); bin != "" {
		// VP owns project configuration, ignore rules, and safe lint fixes.
		// Its suggestions and dangerous fix modes are deliberately not used.
		run("vite-plus", web, bin, "check", "--fix")
		if hasSvelteFiles(web) {
			run("svelte-format", web, localBin(web, "prettier"), "--plugin", "prettier-plugin-svelte", "--write", "src/**/*.svelte")
		}
	} else {
		run("prettier", web, localBin(web, "prettier"), "--write", ".")
		run("eslint", web, localBin(web, "eslint"), "--fix", ".")
	}
	return checks
}
