package generator

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// cliParameters is build-time projection of the already resolved input schema.
// No command hierarchy or application configuration is inferred here.
func cliParameters(data []byte) (string, string, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return "", "", err
	}
	var properties map[string]json.RawMessage
	var required []string
	if err := json.Unmarshal(root["properties"], &properties); err != nil && string(root["properties"]) != "" {
		return "", "", err
	}
	if len(root["required"]) > 0 {
		if err := json.Unmarshal(root["required"], &required); err != nil {
			return "", "", err
		}
	}
	var defs map[string]json.RawMessage
	if len(root["$defs"]) > 0 {
		if err := json.Unmarshal(root["$defs"], &defs); err != nil {
			return "", "", err
		}
	}
	names := make([]string, 0, len(properties))
	flags := map[string]bool{"help": true}
	for name := range properties {
		names = append(names, name)
		flags[name] = true
	}
	sort.Strings(names)
	inputFlag := "input-json"
	for flags[inputFlag] {
		inputFlag += "-json"
	}
	flags[inputFlag] = true
	var parameters strings.Builder
	for _, name := range names {
		var property struct {
			Type, Description, Ref string
			Enum                   []json.RawMessage
		}
		// JSON's $ref spelling is not a Go field name.
		var node map[string]json.RawMessage
		if err := json.Unmarshal(properties[name], &node); err != nil {
			return "", "", err
		}
		_ = json.Unmarshal(node["type"], &property.Type)
		_ = json.Unmarshal(node["description"], &property.Description)
		_ = json.Unmarshal(node["enum"], &property.Enum)
		_ = json.Unmarshal(node["$ref"], &property.Ref)
		if property.Ref != "" {
			var resolved struct {
				Type string
				Enum []json.RawMessage
			}
			_ = json.Unmarshal(defs[strings.TrimPrefix(property.Ref, "#/$defs/")], &resolved)
			property.Type, property.Enum = resolved.Type, resolved.Enum
		}
		kind := "json"
		if property.Type == "string" {
			kind = "string"
		}
		if property.Type == "boolean" {
			kind = "bool"
		}
		if len(property.Enum) > 0 && len(property.Enum[0]) > 0 && property.Enum[0][0] == '"' {
			kind = "string"
		}
		flag := name
		if flag == "help" {
			flag = "arg-help"
			for flags[flag] {
				flag = "arg-" + flag
			}
			flags[flag] = true
		}
		isRequired := false
		for _, r := range required {
			if r == name {
				isRequired = true
			}
		}
		fmt.Fprintf(&parameters, "{Name:%q, Flag:%q, Description:%q, Kind:%q, Required:%t},\n", name, flag, property.Description, kind, isRequired)
	}
	return parameters.String(), inputFlag, nil
}

// cliDocumentation renders reachable contract documentation for command help.
// Detailed help is human-readable; the MCP retains its JSON Schemas unchanged.
func cliDocumentation(data []byte) string {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return ""
	}
	defs, _ := root["$defs"].(map[string]any)
	var out strings.Builder
	var describe func(map[string]any, string, int)
	describe = func(node map[string]any, path string, depth int) {
		description, _ := node["description"].(string)
		if ref, ok := node["$ref"].(string); ok {
			if resolved, ok := defs[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any); ok {
				node = resolved
				if description == "" {
					description, _ = node["description"].(string)
				}
			}
		}
		kind, _ := node["type"].(string)
		if _, ok := node["enum"]; ok {
			kind = "enum"
		}
		if _, ok := node["anyOf"]; ok {
			kind = "nullable value"
		}
		if path != "" {
			fmt.Fprintf(&out, "%s%s", strings.Repeat("  ", depth), path)
			if kind != "" {
				fmt.Fprintf(&out, " (%s)", kind)
			}
			if description != "" {
				fmt.Fprintf(&out, ": %s", description)
			}
			out.WriteByte('\n')
		} else if description != "" {
			out.WriteString(description + "\n")
		}
		if depth >= 4 {
			return
		}
		if properties, ok := node["properties"].(map[string]any); ok {
			names := make([]string, 0, len(properties))
			for name := range properties {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				if child, ok := properties[name].(map[string]any); ok {
					describe(child, name, depth+1)
				}
			}
		}
		if item, ok := node["items"].(map[string]any); ok {
			describe(item, "item", depth+1)
		}
	}
	describe(root, "", 0)
	return strings.TrimSpace(out.String())
}
