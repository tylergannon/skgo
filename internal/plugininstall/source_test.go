package plugininstall

import (
	"os"
	"path/filepath"
	"testing"
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
