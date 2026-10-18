package gen

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEntriesDeclarationRequiresRawRouteEntryParams(t *testing.T) {
	for _, tc := range []struct{ name, file, result, params, want string }{
		{"page", "page.server.go", "[]EntryParams", "", ""},
		{"endpoint", "server.go", "[]EntryParams", "", ""},
		{"request params", "page.server.go", "[]RouteParams", "", "generated EntryParams"},
		{"event", "page.server.go", "[]EntryParams", "value int", "func()"},
		{"layout", "layout.server.go", "[]EntryParams", "", "belongs in page.server.go or server.go"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, gp, p, _ := markerDeclaration(t, tc.file, `package route
import "github.com/tylergannon/skgo"
type EntryParams struct{Slug string}
type RouteParams struct{}
func entries(`+tc.params+`)(`+tc.result+`,error){return nil,nil}
var _=skgo.Entries(entries)
`)
			path := filepath.Join(gp.dir, tc.file)
			stub := filepath.Join(gp.dir, "+page.server.ts")
			if tc.file == "server.go" {
				stub = filepath.Join(gp.dir, "+server.ts")
			}
			err := a.scanEntries(gp, p, p.Syntax[0], path, "src/routes/item/+page.server.ts", stub)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("error=%v want %q", err, tc.want)
				}
				return
			}
			if err != nil || len(a.entries) != 1 || a.entries[0].name != "entries" {
				t.Fatalf("entries=%v error=%v", a.entries, err)
			}
			if err = a.scanEntries(gp, p, p.Syntax[0], path, "src/routes/item/+page.server.ts", stub); err == nil || !strings.Contains(err.Error(), "entries declared twice") {
				t.Fatalf("duplicate error=%v", err)
			}
		})
	}
}
