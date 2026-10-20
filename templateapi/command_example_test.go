package templateapi_test

import (
	"context"
	"fmt"
	"os"

	"github.com/tylergannon/skgo/templateapi"
)

type greetingPlugin struct{}

func (greetingPlugin) Describe() templateapi.Descriptor {
	return templateapi.Descriptor{ID: "greeting", Version: "v1.0.0", SkgoVersion: "v0.41.0", Command: "Print a greeting"}
}
func (greetingPlugin) RunCommand(ctx context.Context, r templateapi.CommandRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(r.Args) != 1 {
		return fmt.Errorf("usage: skgo plugin greeting NAME")
	}
	_, err := fmt.Fprintf(r.Stdout, "Hello, %s!\n", r.Args[0])
	return err
}
func ExampleCommandPlugin() {
	var p templateapi.CommandPlugin = greetingPlugin{}
	_ = p.RunCommand(context.Background(), templateapi.CommandRequest{Args: []string{"Kit"}, Stdout: os.Stdout})
	// Output: Hello, Kit!
}
