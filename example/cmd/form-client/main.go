// Command form-client submits the example's declared Optional Form through its
// generated Go client to a running example server.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/tylergannon/polytype"
	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/internal/skgo/client"
	optional "github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf5xxa5djn5xgc3a"
)

func main() {
	baseURL := flag.String("base-url", "http://127.0.0.1:8080", "running example server")
	name := flag.String("name", "", "Form name")
	var count polytype.Optional[int]
	var enabled polytype.Optional[bool]
	var label polytype.Optional[string]
	flag.Func("count", "include an integer, including zero", func(value string) error {
		n, err := strconv.Atoi(value)
		if err == nil {
			count = polytype.Optional[int]{Present: true, Value: n}
		}
		return err
	})
	flag.Func("enabled", "include a boolean, including false", func(value string) error {
		b, err := strconv.ParseBool(value)
		if err == nil {
			enabled = polytype.Optional[bool]{Present: true, Value: b}
		}
		return err
	})
	flag.Func("label", "include a string, including empty", func(value string) error {
		label = polytype.Optional[string]{Present: true, Value: value}
		return nil
	})
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "form-client: unexpected positional arguments")
		os.Exit(2)
	}
	forms := client.Client{FormClient: skgo.FormClient{BaseURL: *baseURL}}
	result, err := forms.Submit(context.Background(), optional.Input{
		Name: *name, Count: count, Enabled: enabled, Label: label,
	})
	if err != nil {
		var invalid *skgo.Invalid
		if errors.As(err, &invalid) {
			_ = json.NewEncoder(os.Stdout).Encode(invalid.Issues)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "form-client:", err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, "form-client:", err)
		os.Exit(1)
	}
}
