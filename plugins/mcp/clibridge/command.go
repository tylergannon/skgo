// Package clibridge supplies argument handling for generated Cobra commands.
// Applications own command composition, configuration and authentication.
package clibridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
	"github.com/tylergannon/skgo/plugins/mcp/remoteclient"
	"github.com/tylergannon/skgo/plugins/mcp/wirejson"
)

// Parameter describes one top-level input field. Structured values use JSON.
type Parameter struct {
	Name, Flag, Description, Kind string
	Required                      bool
}

// Spec is generated from the same resolved contract as the typed client.
type Spec struct {
	Name, Description, InputDocumentation, OutputDocumentation, InputFlag string
	Parameters                                                            []Parameter
	ObjectInput, NoInput, Mutation                                        bool
}

// Command constructs one independent command. It never adds parents or shared
// flags. The caller may rename, arrange or replace the returned command.
func Command(spec Spec, call func(context.Context, any) (any, error)) *cobra.Command {
	short, _, _ := strings.Cut(spec.Description, "\n")
	long := spec.Description + "\n\nResults are JSON objects with a result property."
	if !spec.NoInput {
		long += "\nString flags accept @path for file contents or @@text for literal @text. Files are UTF-8 and are not trimmed or expanded again.\nUse --" + spec.InputFlag + " for a complete JSON argument object (also accepts @path). Its string values stay literal.\n\nInput:\n" + spec.InputDocumentation
	}
	long += "\n\nResult:\n" + spec.OutputDocumentation
	cmd := &cobra.Command{Use: spec.Name, Short: short, Long: long, Args: cobra.NoArgs}
	values := map[string]*string{}
	bools := map[string]*bool{}
	for _, p := range spec.Parameters {
		help := p.Description
		if p.Required {
			help += " (required)"
		}
		if p.Kind == "json" {
			help += " (JSON value; use the complete JSON input for explicit nulls)"
		}
		if p.Kind == "bool" {
			bools[p.Name] = cmd.Flags().Bool(p.Flag, false, help)
		} else {
			values[p.Name] = cmd.Flags().String(p.Flag, "", help)
		}
	}
	var input *string
	if !spec.NoInput {
		input = cmd.Flags().String(spec.InputFlag, "", "Complete JSON argument object, or @path to read one; cannot be combined with field flags")
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		var raw []byte
		if input != nil && cmd.Flags().Changed(spec.InputFlag) {
			for _, p := range spec.Parameters {
				if cmd.Flags().Changed(p.Flag) {
					return fmt.Errorf("--%s cannot be combined with --%s", spec.InputFlag, p.Flag)
				}
			}
			text, err := ExpandString(*input)
			if err != nil {
				return err
			}
			raw = []byte(text)
		} else {
			fields := map[string]json.RawMessage{}
			for _, p := range spec.Parameters {
				if !cmd.Flags().Changed(p.Flag) {
					if p.Required {
						return fmt.Errorf("required flag --%s is missing; see --help", p.Flag)
					}
					continue
				}
				var value []byte
				switch p.Kind {
				case "bool":
					value, _ = json.Marshal(*bools[p.Name])
				case "string":
					text, err := ExpandString(*values[p.Name])
					if err != nil {
						return fmt.Errorf("--%s: %w", p.Flag, err)
					}
					value, _ = json.Marshal(text)
				default:
					value = []byte(*values[p.Name])
					if !json.Valid(value) {
						return fmt.Errorf("--%s must be a JSON value; see --help", p.Flag)
					}
				}
				fields[p.Name] = value
			}
			var err error
			raw, err = json.Marshal(fields)
			if err != nil {
				return err
			}
		}
		arg, err := wirejson.Arguments(raw, spec.ObjectInput, spec.NoInput)
		if err != nil {
			return fmt.Errorf("invalid input: %w", err)
		}
		value, err := call(cmd.Context(), arg)
		if err != nil {
			if spec.Mutation && !errors.Is(err, wirejson.ErrInvalidInput) && !errors.Is(err, remoteclient.ErrNotSent) {
				return fmt.Errorf("%w; the command may have taken effect. Read current state before repeating it", err)
			}
			return err
		}
		value, err = wirejson.Value(value)
		if err != nil {
			if spec.Mutation {
				return fmt.Errorf("%w; the command completed. Do not repeat it to recover the result", err)
			}
			return err
		}
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"result": value}); err != nil {
			if spec.Mutation {
				return fmt.Errorf("%w; the command completed. Do not repeat it to recover the output", err)
			}
			return err
		}
		return nil
	}
	return cmd
}

// ExpandString applies the CLI's one-pass @file / @@literal convention.
// MCP and direct client inputs do not use this function.
func ExpandString(value string) (string, error) {
	if strings.HasPrefix(value, "@@") {
		return value[1:], nil
	}
	if !strings.HasPrefix(value, "@") {
		if !utf8.ValidString(value) {
			return "", fmt.Errorf("string input must be UTF-8")
		}
		return value, nil
	}
	data, err := os.ReadFile(value[1:])
	if err != nil {
		return "", fmt.Errorf("read file %q: %w; use @@ to send a literal string beginning with @", value[1:], err)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("file %q must contain UTF-8 text", value[1:])
	}
	return string(data), nil
}
