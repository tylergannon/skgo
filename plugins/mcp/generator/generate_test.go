package generator

import (
	"context"
	"encoding/json"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	tg "github.com/tylergannon/polytype/typegrammar"
	"github.com/tylergannon/skgo/templateapi"
)

func TestGeneratorUsesResolvedIdentityAndDescriptions(t *testing.T) {
	r := templateapi.GenerationRequest{Project: templateapi.Project{Module: "example.test/app", Name: "Tasks"}, Selections: []string{"src/tasks.remote.ts#read"}, Model: templateapi.GenerationModel{Roots: []tg.Type{&tg.Scalar{Kind: tg.String}}, Remotes: []templateapi.RemoteOperation{{Name: "read", Description: "Read the current task.", Selection: "src/tasks.remote.ts#read", ID: "resolved/read", Kind: "query", Input: -1, Output: 0}}}}
	got, err := (Generator{}).Generate(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 3 {
		t.Fatalf("files=%d", len(got.Files))
	}
	for _, f := range got.Files {
		if _, err := parser.ParseFile(token.NewFileSet(), f.Path, f.Content, parser.AllErrors); err != nil {
			t.Fatal(err)
		}
	}
	client := string(got.Files[1].Content)
	if !strings.Contains(client, `"resolved/read"`) || !strings.Contains(client, "Read the current task.") {
		t.Fatalf("missing model data: %s", client)
	}
	r.Selections = nil
	if _, err := (Generator{}).Generate(context.Background(), r); err == nil {
		t.Fatal("accepted implicit all-remotes exposure")
	}
	r.Selections = []string{"src/tasks.remote.ts#read"}
	r.Model.Remotes[0].Kind = "form"
	if _, err := (Generator{}).Generate(context.Background(), r); err == nil {
		t.Fatal("accepted form")
	}
}

func TestSchemaPreservesFieldDocumentationAndPresence(t *testing.T) {
	root := &tg.Object{Fields: []tg.Field{{JSONName: "title", Description: "The task title.", Value: &tg.Required{Type: &tg.Scalar{Kind: tg.String}}}, {JSONName: "note", Value: &tg.Optional{Type: &tg.Scalar{Kind: tg.String}}}, {JSONName: "owner", Value: &tg.Nullable{Type: &tg.Scalar{Kind: tg.String}}}}}
	data, err := (schemas{}).document(root, "")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Required             []string
		Properties           map[string]map[string]any
		AdditionalProperties bool
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.Required, ",") != "title,owner" || s.Properties["title"]["description"] != "The task title." || s.AdditionalProperties {
		t.Fatalf("schema=%s", data)
	}
}
