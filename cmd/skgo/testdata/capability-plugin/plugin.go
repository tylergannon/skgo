package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tylergannon/skgo/templateapi"
)

var skgoVersion = "(devel)"

type generator struct{}
type commander struct{}
type both struct {
	generator
	commander
}

func descriptor(generate, command bool) templateapi.Descriptor {
	id := os.Getenv("SKGO_FIXTURE_ID")
	if id == "" {
		id = "capability-fixture"
	}
	d := templateapi.Descriptor{ID: id, Version: "v1.0.0", SkgoVersion: skgoVersion, Generator: generate}
	if command {
		d.Command = "Print forwarded arguments and input"
	}
	return d
}
func (generator) Describe() templateapi.Descriptor { return descriptor(true, false) }
func (commander) Describe() templateapi.Descriptor { return descriptor(false, true) }
func (both) Describe() templateapi.Descriptor      { return descriptor(true, true) }
func SKGoPluginV1() templateapi.Plugin {
	switch os.Getenv("SKGO_FIXTURE_MODE") {
	case "generator":
		return generator{}
	case "command":
		return commander{}
	default:
		return both{}
	}
}
func (generator) Generate(ctx context.Context, r templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
	if err := ctx.Err(); err != nil {
		return templateapi.GenerationResult{}, err
	}
	if r.Project.Name != "CapabilityApp" {
		return templateapi.GenerationResult{}, fmt.Errorf("wrong application name %q", r.Project.Name)
	}
	if r.Options["fail"] == "true" {
		return templateapi.GenerationResult{}, errors.New("fixture generation failed")
	}
	out := r.Options["out"]
	if out == "" {
		out = "generated/fixture.txt"
	}
	return templateapi.GenerationResult{Files: []templateapi.GeneratedFile{{Path: out, Content: []byte(r.Project.Module + "\n" + r.Options["value"] + "\n")}}}, nil
}
func (commander) RunCommand(ctx context.Context, r templateapi.CommandRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(r.Args) > 0 && r.Args[0] == "fail" {
		return errors.New("fixture command failed")
	}
	input, err := io.ReadAll(r.Stdin)
	if err != nil {
		return err
	}
	fmt.Fprintf(r.Stdout, "%s\n%s\n%s", r.Project.Module, strings.Join(r.Args, "|"), input)
	fmt.Fprintln(r.Stderr, "fixture stderr")
	return nil
}
func main() {}
