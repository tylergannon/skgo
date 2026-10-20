//go:build skgo_qualification

package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

// TestAMigratedLinkTreeCompilesRouteEmbeds starts from the layout an earlier
// skgo wrote (a boundary go.mod in the route tree, link directories holding a
// symlink per Go file) and checks that one sync leaves route packages that
// compile with //go:embed: in the route root, and under directories Go cannot
// spell. Each route's own test states the bytes it embeds.
func TestAMigratedLinkTreeCompilesRouteEmbeds(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	embedder := func(dir, pkg, pattern, want string) {
		write(dir+"/embed.go", "package "+pkg+"\n\nimport \"embed\"\n\n//go:embed "+pattern+"\nvar files embed.FS\n")
		write(dir+"/embed_test.go", "package "+pkg+"\n\nimport \"testing\"\n\nfunc TestEmbedded(t *testing.T) {\n"+
			"\tdata, err := files.ReadFile(\"greeting.txt\")\n"+
			"\tif err != nil || string(data) != "+`"`+want+`\n"`+" {\n\t\tt.Fatalf(\"%q, %v\", data, err)\n\t}\n}\n")
		write(dir+"/greeting.txt", want+"\n")
	}
	write("go.mod", "module example.com/app\n\ngo 1.27.1\n")
	embedder("web/src/routes", "routes", "greeting.txt", "root-172")
	embedder("web/src/routes/(files)/[name]", "name", "*.txt", "slug-172")

	// The layout being migrated, as an earlier skgo committed it.
	write("web/src/routes/go.mod", boundaryMarker+"\n\nmodule example.com/app/web/src/routes\n")
	write("generated/links.json", `{"schemaVersion":1,"linkRoot":"generated/_links","boundary":{"path":"web/src/routes/go.mod","owned":true},"links":[]}`)
	for dir, enc := range map[string]string{
		"web/src/routes":                encodeLinkName("src/routes"),
		"web/src/routes/(files)/[name]": encodeLinkName("src/routes/(files)/[name]"),
	} {
		link := filepath.Join(root, "generated", linkRootName, enc)
		if err := os.MkdirAll(link, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"embed.go", "embed_test.go"} {
			target, err := filepath.Rel(link, filepath.Join(root, filepath.FromSlash(dir), name))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(link, name)); err != nil {
				t.Fatal(err)
			}
		}
	}

	cfg := fixtureConfig(Config{Web: filepath.Join(root, "web"), Out: filepath.Join(root, "generated"), Logf: func(string, ...any) {}})
	if err := linksFor(t, cfg).sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "web", "src", "routes", "go.mod")); !os.IsNotExist(err) {
		t.Fatalf("the boundary go.mod an earlier skgo wrote survived migration: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	mod, err := modfile.Parse("go.mod", raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(mod.Ignore) != 1 || mod.Ignore[0].Path != "./web/src/routes" {
		t.Fatalf("go.mod ignores %v, want ./web/src/routes:\n%s", mod.Ignore, raw)
	}
	inventory, err := os.ReadFile(filepath.Join(root, "generated", "links.json"))
	if err != nil || strings.Contains(string(inventory), "boundary") {
		t.Fatalf("links.json still records a boundary: %s, %v", inventory, err)
	}

	// One go command: the route packages compile and their tests pass through
	// the links, and ./... finds nothing to trip over in the route tree.
	links := []string{
		"./generated/_links/" + encodeLinkName("src/routes"),
		"./generated/_links/" + encodeLinkName("src/routes/(files)/[name]"),
	}
	cmd := exec.Command("go", append([]string{"test", "-count=1", "./..."}, links...)...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test: %v\n%s", err, out)
	}
	for _, link := range links {
		if !strings.Contains(string(out), "ok  \texample.com/app/"+strings.TrimPrefix(link, "./")) {
			t.Fatalf("go test did not pass %s:\n%s", link, out)
		}
	}
}
