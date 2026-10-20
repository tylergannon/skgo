package templateapi_test

import (
	"context"
	"fmt"
	"strings"

	"github.com/tylergannon/skgo/templateapi"
)

// A plugin's ordinary factory can return this value as templateapi.Plugin.
// Generator is an optional capability on that same value.
type operationList struct{}

func (operationList) Describe() templateapi.Descriptor {
	return templateapi.Descriptor{ID: "operation-list", Version: "v1.0.0", Generator: true}
}
func (operationList) Generate(ctx context.Context, request templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
	if err := ctx.Err(); err != nil {
		return templateapi.GenerationResult{}, err
	}
	var text strings.Builder
	for _, remote := range request.Model.Remotes {
		fmt.Fprintf(&text, "%s %s\n", remote.Kind, remote.ID)
	}
	return templateapi.GenerationResult{Files: []templateapi.GeneratedFile{{Path: "generated/operations.txt", Content: []byte(text.String())}}}, nil
}

func ExampleGenerator() {
	var generator templateapi.Generator = operationList{}
	result, err := generator.Generate(context.Background(), templateapi.GenerationRequest{
		Model: templateapi.GenerationModel{Remotes: []templateapi.RemoteOperation{
			{Name: "read", ID: "worolc/read", Kind: "query", Input: -1, Output: 0},
			{Name: "save", ID: "worolc/save", Kind: "command", Input: 0, Output: 0},
		}},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Files[0].Path)
	fmt.Print(string(result.Files[0].Content))
	// Output:
	// generated/operations.txt
	// query worolc/read
	// command worolc/save
}
