package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/tylergannon/skgo/internal/envspec"
)

func environmentCommand(args []string, in io.Reader, out io.Writer) error {
	flags := flag.NewFlagSet("env", flag.ContinueOnError)
	schemaFile := flags.String("schema", "skgo.env.json", "generated environment declaration")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("skgo env accepts no positional arguments")
	}
	data, err := os.ReadFile(*schemaFile)
	if err != nil {
		return err
	}
	var schema envspec.Schema
	if err = json.Unmarshal(data, &schema); err != nil {
		return fmt.Errorf("skgo: invalid environment schema")
	}
	var raw map[string]string
	if err = json.NewDecoder(in).Decode(&raw); err != nil {
		return fmt.Errorf("skgo: invalid environment input")
	}
	values, err := schema.Resolve(func(name string) (string, bool) { value, ok := raw[name]; return value, ok }, false)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(values)
}
