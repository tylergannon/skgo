// Package generator emits typed Go remote clients and an MCP stdio entrypoint
// from SKGO's resolved model. It never reloads application packages.
package generator

import (
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"sort"
	"strconv"
	"strings"
	"unicode"

	codec "github.com/tylergannon/polytype/devalue/codegen"
	tg "github.com/tylergannon/polytype/typegrammar"
	"github.com/tylergannon/skgo/templateapi"
)

type Generator struct{}

// RemoteMarker opts into core's publication-marker selection before projection.
func (Generator) RemoteMarker() string {
	return "github.com/tylergannon/skgo/plugins/mcp/mcppublish#Tool"
}

func (Generator) Generate(ctx context.Context, r templateapi.GenerationRequest) (templateapi.GenerationResult, error) {
	var result templateapi.GenerationResult
	if r.RemoteMarker != (Generator{}).RemoteMarker() {
		return result, fmt.Errorf("MCP requires a host that applied its publication marker filter; upgrade the SKGO host")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if len(r.Model.Remotes) == 0 {
		return result, fmt.Errorf("MCP needs at least one remote marked with mcppublish.Tool")
	}
	s := schemas{r.Model.Definitions}
	imports := map[string]string{}
	var typeName func(tg.Type) (string, error)
	named := func(n tg.Name) (string, error) {
		if !ast.IsExported(n.Name) {
			return "", fmt.Errorf("%s must be an exported contract type", n)
		}
		alias := imports[n.PackagePath]
		if alias == "" {
			alias = fmt.Sprintf("contract%d", len(imports))
			imports[n.PackagePath] = alias
		}
		return alias + "." + n.Name, nil
	}
	typeName = func(t tg.Type) (string, error) {
		switch t := t.(type) {
		case *tg.Ref:
			return named(t.Target)
		case *tg.Enum:
			return named(t.GoType)
		case *tg.Scalar:
			return string(t.Kind), nil
		case *tg.Time:
			imports["time"] = "time"
			return "time.Time", nil
		case *tg.Pointer:
			v, err := typeName(t.Element)
			return "*" + v, err
		case *tg.Slice:
			v, err := typeName(t.Element)
			return "[]" + v, err
		case *tg.Array:
			v, err := typeName(t.Element)
			return fmt.Sprintf("[%d]%s", t.Length, v), err
		default:
			return "", fmt.Errorf("use an exported named contract type for %T", t)
		}
	}
	var methods, tools strings.Builder
	if len(r.Selections) != 0 {
		return result, fmt.Errorf("MCP uses mcppublish.Tool markers; remove configured remote selections")
	}
	for _, op := range r.Model.Remotes {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if op.Kind != "query" && op.Kind != "command" || op.Transported || op.Output < 0 || op.Output >= len(r.Model.Roots) || op.Input < -1 || op.Input >= len(r.Model.Roots) {
			return result, fmt.Errorf("%s: MCP supports ordinary queries and commands with projected wire types", op.Selection)
		}
		if strings.TrimSpace(op.Description) == "" {
			return result, fmt.Errorf("%s needs a Go function documentation comment", op.Selection)
		}
		runes := []rune(op.Name)
		if len(runes) == 0 {
			return result, fmt.Errorf("empty operation name")
		}
		runes[0] = unicode.ToUpper(runes[0])
		method := string(runes)
		output := r.Model.Roots[op.Output]
		outType, err := typeName(output)
		if err != nil {
			return result, err
		}
		var input tg.Type
		inType, parameter, argument, encode := "", "", "", "input := any(devalue.Undefined)\n"
		wrapper := ""
		if op.Input >= 0 {
			input = r.Model.Roots[op.Input]
			inType, err = typeName(input)
			if err != nil {
				return result, err
			}
			parameter, argument = ", in "+inType, ", in"
			encode = fmt.Sprintf("input, err := EncodeRoot%d(in); if err != nil { return zero, err }\n", op.Input)
			if !s.object(input) {
				wrapper = "argument"
			}
		}
		inputSchema, err := s.document(input, wrapper)
		if err != nil {
			return result, fmt.Errorf("%s input: %w", op.Selection, err)
		}
		outputSchema, err := s.document(output, "result")
		if err != nil {
			return result, fmt.Errorf("%s output: %w", op.Selection, err)
		}
		fmt.Fprintf(&methods, "// %s\nfunc (c *Client) %s(ctx context.Context%s) (%s, error) {\nvar zero %s\n%sraw, err := c.Remote.Call(ctx, %q, %q, input)\nif err != nil { return zero, err }; value, err := DecodeRoot%d(raw); if err != nil { return zero, fmt.Errorf(\"result contract: %%w\", err) }; return value, nil\n}\n", strings.ReplaceAll(op.Description, "\n", "\n// "), method, parameter, outType, outType, encode, op.ID, op.Kind, op.Output)
		fmt.Fprintf(&tools, "bridge.Tool(%q, %q, json.RawMessage(%s), json.RawMessage(%s), %t, %t, %t, func(ctx context.Context, arg any)(any,error){\n", op.Name, op.Description, strconv.Quote(string(inputSchema)), strconv.Quote(string(outputSchema)), s.object(input), op.Input < 0, op.Kind == "command")
		if op.Input >= 0 {
			fmt.Fprintf(&tools, "in, err := DecodeRoot%d(arg); if err != nil { return nil, fmt.Errorf(\"%%w: %%v\", bridge.ErrInvalidInput, err) }\n", op.Input)
		}
		fmt.Fprintf(&tools, "value, err := c.%s(ctx%s); if err != nil { return nil, err }; return EncodeRoot%d(value)\n}),\n", method, argument, op.Output)
	}
	for _, d := range r.Model.Definitions {
		if _, err := named(d.Name); err != nil {
			return result, err
		}
	}
	// Codecs manage their own imports; client imports only its public signature types.
	usedImports := map[string]string{}
	for pkg, alias := range imports {
		if strings.Contains(methods.String(), alias+".") {
			usedImports[pkg] = alias
		}
	}
	client := `// Code generated by the SKGO MCP plugin. DO NOT EDIT.
package mcpclient
import (
 "context"
 "encoding/json"
 "fmt"
 "github.com/mark3labs/mcp-go/server"
 "github.com/tylergannon/devalue/v5"
 "github.com/tylergannon/skgo/plugins/mcp/bridge"
 "github.com/tylergannon/skgo/plugins/mcp/remoteclient"
`
	paths := []string{}
	for pkg := range usedImports {
		paths = append(paths, pkg)
	}
	sort.Strings(paths)
	for _, pkg := range paths {
		client += fmt.Sprintf("%s %q\n", usedImports[pkg], pkg)
	}
	client += ")\ntype Client struct { Remote remoteclient.Client }\n" + methods.String() + "\nfunc (c *Client) Tools() []server.ServerTool { return []server.ServerTool{\n" + tools.String() + "} }\n"
	if !strings.Contains(methods.String(), "devalue.") {
		client = strings.Replace(client, " \"github.com/tylergannon/devalue/v5\"\n", "", 1)
	}
	codecs, err := codec.Generate(r.Model.Definitions, r.Model.Roots, codec.Options{PackageName: "mcpclient", ImportPath: r.Project.Module + "/internal/mcpclient"})
	if err != nil {
		return result, err
	}
	for _, file := range []templateapi.GeneratedFile{{Path: "internal/mcpclient/codec_gen.go", Content: codecs}, {Path: "internal/mcpclient/client_gen.go", Content: []byte(client)}} {
		data, err := format.Source(file.Content)
		if err != nil {
			return result, fmt.Errorf("format %s: %w", file.Path, err)
		}
		file.Content = data
		result.Files = append(result.Files, file)
	}
	return result, nil
}
