package gen

import (
	"go/format"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedFormClientContract(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	a, gp := emitterPackage(t, root, "package data\ntype Input struct { Name string }\ntype Receipt struct { Label string }\n")
	fn := &remoteFn{name: "submit", module: "src/lib/todos.remote.ts", handler: "remote_submit", kind: kindForm, goPkg: gp, in: gp.pkg.Types.Scope().Lookup("Input").Type(), out: gp.pkg.Types.Scope().Lookup("Receipt").Type()}
	a.remotes = []*remoteFn{fn}
	if err := a.writeFormClients(); err != nil {
		t.Fatal(err)
	}
	source := a.cfg.generation.files[filepath.Join(a.cfg.Out, "client", generatedGoFile)]

	const method = `func (c Client) Submit(ctx context.Context, in skgo0.SkgoArg_submit) (skgo0.SkgoOut_submit, error) {
 return skgo.SubmitForm(ctx, c.FormClient, "worolc/submit", in, DecodeRoot0)
}`
	wantMethod, err := format.Source([]byte("package client\n" + method))
	if err != nil {
		t.Fatal(err)
	}
	// The literal route hash is shared with Kit's captured src/lib/todos.remote.ts fixture.
	if !strings.Contains(source, strings.TrimPrefix(string(wantMethod), "package client\n\n")) {
		t.Fatalf("client lost its typed request/response delegation:\n%s", source)
	}

	stub, err := a.stubSignature(fn)
	const want = "export const submit = form('unchecked', (_arg: Input): Receipt => unimplemented());\n"
	if err != nil || stub != want {
		t.Fatalf("stub=%q, %v; want %q", stub, err, want)
	}
}
