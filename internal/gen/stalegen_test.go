package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func linkGeneratorKit(root, app string) error {
	src := filepath.Join(root, "example")
	kit := filepath.Join(src, "web", "node_modules", "@sveltejs", "kit")
	if _, err := os.Stat(filepath.Join(kit, "src", "utils", "routing.js")); err != nil {
		return fmt.Errorf("generator example fixtures require the installed pinned Kit source: %w", err)
	}
	link := filepath.Join(app, "web", "node_modules", "@sveltejs", "kit")
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		return err
	}
	if err := os.Symlink(kit, link); err != nil {
		return err
	}
	return nil
}

// copySandboxTree copies src to dst, skipping the src-relative paths in skip.
func copySandboxTree(src, dst string, skip map[string]bool) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel != "." && skip[rel] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 || !d.Type().IsRegular() {
			return nil
		}
		return copySandboxFile(path, target)
	})
}

func copySandboxFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// rewriteSkgoReplace points the sandbox's `replace` directive back at the
// real skgo module, which is no longer one directory up once the app has been
// copied out from under it.
func rewriteSkgoReplace(gomod, root string) error {
	raw, err := os.ReadFile(gomod)
	if err != nil {
		return err
	}
	const relative = "replace github.com/tylergannon/skgo => ../"
	if !strings.Contains(string(raw), relative) {
		return &fixtureError{gomod: gomod, want: relative}
	}
	rewritten := strings.Replace(string(raw), relative, "replace github.com/tylergannon/skgo => "+root, 1)
	return os.WriteFile(gomod, []byte(rewritten), 0o644)
}

type fixtureError struct {
	gomod, want string
}

func (e *fixtureError) Error() string {
	return e.gomod + " no longer contains " + e.want + "; update this test"
}
