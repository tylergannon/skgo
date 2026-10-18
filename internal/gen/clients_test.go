package gen

import (
	"strings"
	"testing"
)

func TestGeneratedFormClientContract(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, `package data
import("context";"github.com/tylergannon/skgo";"example.com/app/generated/params")
type Input struct { Name string }
type Receipt struct { Label string }
func submit(context.Context,params.RequestEvent,Input)(Receipt,error){return Receipt{},nil}
var _=skgo.Form(submit)
`, nil)
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	source := readFixtureFile(t, root, "app/generated/client/skgo_gen.go")
	for _, want := range []string{"func (c Client) Submit(", "return skgo.SubmitForm(", `at+"/Label"`, "dvString("} {
		if !strings.Contains(source, want) {
			t.Fatalf("generated client lacks %q:\n%s", want, source)
		}
	}
	stub := readFixtureFile(t, root, "app/web/src/data/data.remote.ts")
	if !strings.Contains(stub, `submit = form('unchecked', (_arg: Input): Receipt => unimplemented())`) {
		t.Fatal(stub)
	}
}
