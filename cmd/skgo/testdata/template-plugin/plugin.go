package main

import (
	"context"
	"fmt"
	"github.com/tylergannon/skgo/templateapi"
	"os"
	"path/filepath"
)

var skgoVersion = "(devel)"

type addon struct{}

func SKGoPluginV1() templateapi.Plugin { return addon{} }
func (addon) Describe() templateapi.Descriptor {
	return templateapi.Descriptor{ID: "external-fixture", Version: "1", SkgoVersion: skgoVersion, Addons: []templateapi.Addon{{Name: "receipt", Summary: "Write a configured receipt", Options: []templateapi.Option{{Name: "text", Help: "Receipt text", Required: true}}}}, Templates: []templateapi.Template{{Name: "receipt-app", Summary: "An application receipt", Options: []templateapi.Option{{Name: "message", Help: "Receipt message", Default: "template receipt"}}, Steps: []templateapi.Step{{Addon: "receipt", Bindings: map[string]string{"text": "message"}}}}}}
}
func (addon) Apply(ctx context.Context, r templateapi.Request) (templateapi.Result, error) {
	if err := ctx.Err(); err != nil {
		return templateapi.Result{}, err
	}
	path := filepath.Join(r.Project.Root, "receipt.txt")
	data := []byte(r.Project.Name + "\n" + r.Project.Module + "\n" + r.Options["text"] + "\n")
	old, err := os.ReadFile(path)
	if err == nil {
		if string(old) == string(data) {
			return templateapi.Result{}, nil
		}
		return templateapi.Result{}, fmt.Errorf("receipt.txt conflicts with existing content")
	}
	if !os.IsNotExist(err) {
		return templateapi.Result{}, err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return templateapi.Result{}, err
	}
	return templateapi.Result{Changed: []string{"receipt.txt"}}, nil
}
func main() {}
