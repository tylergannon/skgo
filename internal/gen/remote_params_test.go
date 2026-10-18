package gen

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandFormEventDiagnostics(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"Command", "Form"} {
		for _, tc := range []struct{ name, signature, want string }{
			{"ctx-only", "ctx context.Context,in string", "A " + strings.ToLower(kind) + " is func(context.Context, params.RequestEvent"},
			{"event-only", "event params.RequestEvent,in string", "A " + strings.ToLower(kind) + " is func(context.Context, params.RequestEvent"},
			{"wrong-params", "ctx context.Context,event skgo.RequestEvent[struct{}, struct{}],in string", "event locals must be example.com/app/internal/app.Locals"},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				a, gp, p, call := markerDeclaration(t, "data.remote.go", `package data
import("context";"github.com/tylergannon/skgo";"example.com/app/generated/params")
var _ = params.Params{}; var _ = context.Background
func old(`+tc.signature+`)(string,error){return in,nil}
var _ = skgo.`+kind+`(old)
`)
				_, _, _, _, err := a.readMarker(gp, p, call, "src/data.remote.ts", "", "", filepath.Join(gp.dir, "data.remote.go"))
				if err == nil || !strings.Contains(err.Error(), "data.remote.go:5:") || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("missing source-located %s migration: %v", kind, err)
				}
				if strings.Contains(err.Error(), "RequestEvent alias") {
					t.Fatalf("%s migration recommends an alias instead of the explicit generic type: %v", kind, err)
				}
			})
		}
	}
}
