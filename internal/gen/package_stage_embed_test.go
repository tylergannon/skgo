package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestStagedEmbedsLeaveUnmatchedTreesAlone(t *testing.T) {
	const declarations = "package main\nimport \"embed\"\n" +
		"//go:embed all:build\nvar all embed.FS\n" +
		"//go:embed plain/* \"space name/*.txt\" `quoted name/*.txt`\nvar glob embed.FS\n" +
		"//go:embed plain\nvar plain embed.FS\n" +
		"//go:embed assets/nested/*.txt\nvar nested embed.FS\n"
	for _, source := range []string{"authored", "replacement", "new"} {
		t.Run(source, func(t *testing.T) {
			dir := t.TempDir()
			files := map[string]string{
				"go.mod": "module embedfixture\n\ngo 1.27.1\n",
				"main.go": `package main
import ("fmt"; "io/fs")
func main() {
    for _, tree := range []fs.FS{all, glob, plain, nested} {
        fs.WalkDir(tree, ".", func(path string, entry fs.DirEntry, err error) error {
            if err != nil { panic(err) }
            if !entry.IsDir() { data, err := fs.ReadFile(tree, path); if err != nil { panic(err) }; fmt.Printf("%s=%s\n", path, data) }
            return nil
        })
    }
    fmt.Println(Generated)
}
`,
				"build/public.txt":               "public",
				"build/.hidden.txt":              "hidden",
				"build/_app/chunk.txt":           "chunk",
				"build/.directory/value.txt":     "dotdir",
				"plain/.direct.txt":              "direct",
				"plain/public.txt":               "plain",
				"plain/deep/.hidden.txt":         "excluded",
				"plain/deep/visible.txt":         "visible",
				"space name/value.txt":           "space",
				"quoted name/value.txt":          "quoted",
				"assets/nested/value.txt":        "nested",
				"assets/unrelated/unneeded.txt":  "unneeded",
				"node_modules/unneeded/data.txt": "unrelated",
			}
			if source == "authored" {
				files["assets.go"] = declarations
			} else if source == "replacement" {
				// The old directive must not force a subtree into the new view.
				files["assets.go"] = "package main\nimport \"embed\"\n//go:embed all:node_modules\nvar old embed.FS\n"
			}
			for name, data := range files {
				path := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cfg := &packages.Config{Dir: dir, Env: append(os.Environ(), "GOWORK=off"), Overlay: map[string][]byte{
				filepath.Join(dir, "generated.go"): []byte("package main\nconst Generated = 42\n"),
			}}
			if source != "authored" {
				cfg.Overlay[filepath.Join(dir, "assets.go")] = []byte(declarations)
			}
			cleanup, err := stagePackageOverlay(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			for _, name := range []string{"node_modules", "assets/unrelated"} {
				info, err := os.Lstat(filepath.Join(cfg.Dir, name))
				if err != nil || info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("unmatched tree %s was materialized: %v, %v", name, info, err)
				}
			}
			cmd := exec.Command("go", "run", ".")
			cmd.Dir, cmd.Env = cfg.Dir, cfg.Env
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("compiled staged embeds: %v\n%s", err, out)
			}
			want := strings.Join([]string{
				"build/.directory/value.txt=dotdir", "build/.hidden.txt=hidden", "build/_app/chunk.txt=chunk", "build/public.txt=public",
				"plain/.direct.txt=direct", "plain/deep/visible.txt=visible", "plain/public.txt=plain", "quoted name/value.txt=quoted", "space name/value.txt=space",
				"plain/deep/visible.txt=visible", "plain/public.txt=plain", "assets/nested/value.txt=nested", "42", "",
			}, "\n")
			if string(out) != want {
				t.Fatalf("embedded values:\n%s\nwant:\n%s", out, want)
			}
			for name, want := range files {
				got, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || string(got) != want {
					t.Fatalf("authored %s changed: %q, %v", name, got, err)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, "generated.go")); !os.IsNotExist(err) {
				t.Fatalf("published temporary generated source: %v", err)
			}
		})
	}
}
