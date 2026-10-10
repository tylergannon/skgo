package plugininstall

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestManifestBoundary(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct{ body, want string }{
		{`{"package":"."}`, "."}, {`{"package":"./template"}`, "./template"},
		{`{}`, ""}, {`null`, ""}, {`{"package":"/tmp/plugin"}`, ""}, {`{"package":"../plugin"}`, ""},
		{`{"package":"./../plugin"}`, ""}, {`{"package":"./a/../b"}`, ""}, {`{"package":"./..."}`, ""},
		{`{"package":"./template","prepare":"shell"}`, ""}, {`{"package":"."} {}`, ""}, {`{"package":"./a\\b"}`, ""},
	} {
		t.Run(tc.body, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(root, "skgo-plugin.json"), []byte(tc.body), 0644); err != nil {
				t.Fatal(err)
			}
			got, err := manifest(root)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("accepted %s as %q", tc.body, got)
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("got %q,%v", got, err)
			}
		})
	}
}

func TestFetchHonorsChecksumDatabaseRefusal(t *testing.T) {
	const modulePath = "github.com/skgo-test/checksum-refusal"
	const refusal = "fixture checksum database refuses this module"
	repo := t.TempDir()
	for name, body := range map[string]string{
		"go.mod":           "module " + modulePath + "\n\ngo 1.27.1\n",
		"skgo-plugin.json": "{\"package\":\".\"}\n",
		"main.go":          "package main\nfunc main() {}\n",
	} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(t.TempDir(), "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	t.Setenv("GIT_AUTHOR_NAME", "Fixture")
	t.Setenv("GIT_AUTHOR_EMAIL", "fixture@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Fixture")
	t.Setenv("GIT_COMMITTER_EMAIL", "fixture@example.com")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
	}
	git("config", "--global", "url."+(&url.URL{Scheme: "file", Path: filepath.ToSlash(repo)}).String()+".insteadOf", "https://"+modulePath)
	git("init", "--quiet")
	git("add", ".")
	git("-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "checksum refusal fixture")
	git("tag", "v1.0.0")

	lookups := make(chan string, 16)
	db := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case lookups <- r.URL.Path:
		default:
		}
		http.Error(w, refusal, http.StatusForbidden)
	}))
	defer db.Close()
	// Use the already-running Go distribution, while isolating all resolver
	// caches and caller policy. Git's static GitHub mapping is rewritten to the
	// local repository; the only HTTP endpoint is this refusing checksum server.
	t.Setenv("PATH", filepath.Join(runtime.GOROOT(), "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GOENV", "off")
	t.Setenv("GOPATH", t.TempDir())
	t.Setenv("GOSUMDB", "sum.golang.org "+db.URL)
	t.Setenv("GONOSUMDB", "")
	t.Setenv("GOPRIVATE", "")
	t.Setenv("GONOPROXY", "")
	t.Setenv("GOINSECURE", "")
	t.Setenv("GOVCS", "*:git")
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var output bytes.Buffer
	s, cleanup, err := fetch(ctx, modulePath+"@v1.0.0", runtime.Version(), &output)
	if cleanup != nil {
		t.Cleanup(cleanup)
	}
	if err == nil || !strings.Contains(err.Error(), refusal) || !strings.Contains(err.Error(), "verifying go.mod") {
		t.Fatalf("checksum refusal was not returned: %v\n%s", err, &output)
	}
	select {
	case path := <-lookups:
		if path != "/lookup/"+modulePath+"@v1.0.0" {
			t.Fatalf("unexpected checksum database request: %q", path)
		}
	default:
		t.Fatal("resolver never queried the configured checksum database")
	}
	if !strings.Contains(output.String(), "Source checksum policy: checksum database sum.golang.org "+db.URL) {
		t.Fatalf("configured checksum policy not named: %s", &output)
	}
	if s.Root != "" || s.Package != "" || strings.Contains(output.String(), "Resolved ") {
		t.Fatalf("refused source reached preparation: %+v\n%s", s, &output)
	}
	entries, err := os.ReadDir(temp)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed fetch left temporary source/cache files: %v, %v", entries, err)
	}
}
func TestReferenceQueries(t *testing.T) {
	for _, tc := range []struct{ ref, path, query string }{
		{"example.com/plugin", "example.com/plugin", "latest"}, {"example.com/plugin@main", "example.com/plugin", "main"}, {"example.com/plugin/v2@v2.3.0", "example.com/plugin/v2", "v2.3.0"},
		{"./local", "", ""}, {"example.com/plugin@", "", ""}, {"example.com/plugin@a@b", "", ""}, {"example.com/plugin@a b", "", ""},
	} {
		p, q, e := parseReference(tc.ref)
		if tc.path == "" {
			if e == nil {
				t.Fatalf("accepted %q", tc.ref)
			}
		} else if e != nil || p != tc.path || q != tc.query {
			t.Fatalf("%s: %s %s %v", tc.ref, p, q, e)
		}
	}
}

func TestCheckoutVerifiesCanonicalHashesAndPreservesNestedSource(t *testing.T) {
	repo := t.TempDir()
	files := map[string]string{
		"go.mod":           "module example.com/source-fixture\n\ngo 1.27.1\n",
		"skgo-plugin.json": "{\"package\":\"./template\"}\n",
		"template/main.go": "package main\nfunc main() {}\n",
		"prepare.sh":       "#!/bin/sh\nprintf prepared\\n\n",
		"nested/go.mod":    "module example.com/nested\n\ngo 1.27.1\n",
		"nested/data.txt":  "nested source required by preparation\n",
	}
	for name, body := range files {
		path := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(filepath.Join(repo, "prepare.sh"), 0755); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
			"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.com",
			"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.com")
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, b)
		}
		return b
	}
	git("init", "--quiet")
	git("add", ".")
	git("-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "literal source fixture")
	commit := strings.TrimSpace(string(git("rev-parse", "HEAD")))
	// Materialization must read the commit, not local checkout modifications.
	if err := os.WriteFile(filepath.Join(repo, "nested/data.txt"), []byte("uncommitted replacement\n"), 0644); err != nil {
		t.Fatal(err)
	}
	tree := filepath.Join(t.TempDir(), "tree")
	if err := extractTree(git("archive", "--format=tar", commit), tree); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(tree, "nested/data.txt")); err != nil || string(b) != "nested source required by preparation\n" {
		t.Fatalf("nested immutable source: %q %v", b, err)
	}
	if st, err := os.Stat(filepath.Join(tree, "prepare.sh")); err != nil || st.Mode().Perm() != 0755 {
		t.Fatalf("tracked executable mode: %v %v", st, err)
	}
	// These hashes were independently calculated from the literal fixture bytes
	// above using the documented h1 SHA-256 name/content format. The module zip
	// excludes the nested Go module; the full Git archive must still retain it.
	const moduleSum = "h1:bQCOkn45jKDM3J1XCurkiSenznAPy+k9IGY047PDCDo="
	const goModSum = "h1:0rxIQfjiB0e0t3tBFJZPDe3reru2cBJ/RAlzJjmobZ4="
	s := source{resolution: resolution{Path: "example.com/source-fixture", Version: "v1.2.3", Sum: moduleSum, GoModSum: goModSum}, Root: tree}
	s.Origin = &struct{ VCS, URL, Hash, Subdir string }{VCS: "git", URL: repo, Hash: commit}
	if err := verifyCheckout(context.Background(), repo, &s); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, diagnostic string }{
		{"module hash", "source checksum mismatch"},
		{"go.mod hash", "go.mod checksum mismatch"},
		{"commit", "checkout commit mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := s
			origin := *s.Origin
			bad.Origin = &origin
			switch tc.name {
			case "module hash":
				bad.Sum = "h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
			case "go.mod hash":
				bad.GoModSum = "h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
			case "commit":
				bad.Origin.Hash = strings.Repeat("0", 40)
			}
			if err := verifyCheckout(context.Background(), repo, &bad); err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("accepted wrong %s: %v", tc.name, err)
			}
		})
	}
}

func TestCommandSeparatesMachineOutputFromDiagnostics(t *testing.T) {
	if mode := os.Getenv("SKGO_TEST_COMMAND_IO"); mode != "" {
		os.Stdout.WriteString("/literal/toolchain/root\n")
		os.Stderr.WriteString("go: downloading the selected toolchain\n")
		if mode == "failure" {
			os.Exit(2)
		}
		os.Exit(0)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"success", "failure"} {
		b, err := command(context.Background(), t.TempDir(), []string{"SKGO_TEST_COMMAND_IO=" + mode}, executable, "-test.run=^TestCommandSeparatesMachineOutputFromDiagnostics$")
		if string(b) != "/literal/toolchain/root\n" {
			t.Fatalf("stderr polluted parsed output: %q", b)
		}
		if mode == "success" && err != nil {
			t.Fatal(err)
		}
		if mode == "failure" && (err == nil || !strings.Contains(err.Error(), "go: downloading the selected toolchain")) {
			t.Fatalf("lost failure diagnostics: %v", err)
		}
	}
}
