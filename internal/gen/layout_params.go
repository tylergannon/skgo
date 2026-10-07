package gen

import (
	"fmt"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"github.com/tylergannon/skgo"
)

func loadParamType(param skgo.ManifestParam, matchers map[string]goParamMatcher) types.Type {
	if param.Matcher != "" {
		return matchers[param.Matcher].out
	}
	return types.Typ[types.String]
}

func writeLoadEvent(body *strings.Builder, cfg Config, domain, kind, populate string) {
	fmt.Fprintf(body, "type %sRequestEvent = skgo.RequestEvent[%s, appstate.%s]\nfunc Skgo%sRequestEvent(event *skgo.Event) %sRequestEvent {\nreturn %sRequestEvent{Event:event,Locals:appstate.LocalsFrom(event.Request().Context()),Params:skgo%s(event)}\n}\nfunc skgo%s(event *skgo.Event) %s {\np := %s{event:event}\n%s\nreturn p\n}\n", kind, domain, cfg.LocalsType, kind, kind, kind, domain, domain, domain, domain, populate)
}

func writeConcreteLoadDomain(body *strings.Builder, imports *fileImports, cfg Config, domain, kind string, params []skgo.ManifestParam, matchers map[string]goParamMatcher) error {
	var accessors, populate strings.Builder
	fmt.Fprintf(body, "// %s stores converted values; only accessors record load dependencies.\ntype %s struct { event *skgo.Event\n", domain, domain)
	fields := map[string]bool{}
	for _, param := range params {
		field := paramField(param.Name)
		if !token.IsIdentifier(field) || fields[field] {
			return fmt.Errorf("skgo: route parameter %q has a conflicting Go field %q", param.Name, field)
		}
		fields[field] = true
		typ := imports.typeExpr(loadParamType(param, matchers))
		helper, result := "LoadParamValue", typ
		if param.Optional {
			helper, result = "OptionalLoadParamValue", "*"+typ
		}
		fmt.Fprintf(body, "value%s %s\n", field, result)
		fmt.Fprintf(&accessors, "func (p %s) %s() %s {skgo.TrackLoadParam(p.event,%q);return p.value%s}\n", domain, field, result, param.Name, field)
		fmt.Fprintf(&populate, "p.value%s = skgo.%s[%s](event,%q)\n", field, helper, typ, param.Name)
	}
	body.WriteString("}\n")
	body.WriteString(accessors.String())
	writeLoadEvent(body, cfg, domain, kind, populate.String())
	return nil
}

// The participating page set is supplied by Kit's manifest builder. Only the
// type widening is ours: Kit's first-matcher-per-key type cannot describe all
// of the Go values that the actual matched page can supply to a layout.
func writeLayoutDomain(body *strings.Builder, imports *fileImports, cfg Config, info *routeLoadParams) error {
	alternatives := map[string][]sharedAlternative{}
	optional := map[string]bool{}
	add := func(param skgo.ManifestParam, matchers map[string]goParamMatcher) {
		typ := loadParamType(param, matchers)
		for _, alt := range alternatives[param.Name] {
			if types.Identical(types.Unalias(alt.typ), types.Unalias(typ)) {
				return
			}
		}
		alternatives[param.Name] = append(alternatives[param.Name], sharedAlternative{typ: typ, name: sharedVariantName(param.Name, typ)})
	}
	for _, param := range info.params {
		add(param, info.matchers)
		optional[param.Name] = param.Optional
	}
	var ids []string
	for id := range info.children {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		child := info.children[id]
		for _, param := range child.params {
			if _, local := optional[param.Name]; !local {
				optional[param.Name] = true
			}
			add(param, child.matchers)
		}
	}
	nameSharedAlternatives(alternatives)
	var keys []string
	for key := range alternatives {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var accessors, populate, unions strings.Builder
	fields := map[string]bool{}
	body.WriteString("// LayoutParams stores this layout's local and participating-page values.\ntype LayoutParams struct { event *skgo.Event\n")
	hasUnion := false
	for _, key := range keys {
		field := sharedParamStem(key)
		if !token.IsIdentifier(field) || fields[field] {
			return fmt.Errorf("skgo: layout parameter %q has a conflicting Go field %q", key, field)
		}
		fields[field] = true
		alts := alternatives[key]
		var result string
		if len(alts) == 1 {
			typ := imports.typeExpr(alts[0].typ)
			helper := "LoadParamValue"
			result = typ
			if optional[key] {
				helper, result = "OptionalLoadParamValue", "*"+typ
			}
			fmt.Fprintf(&populate, "p.value%s = skgo.%s[%s](event,%q)\n", field, helper, typ, key)
		} else {
			hasUnion = true
			result = "Key_" + sharedParamStem(key)
			fmt.Fprintf(&unions, "type %s interface { skgoParam_%x() }\n", result, key)
			sort.Slice(alts, func(i, j int) bool { return alts[i].name < alts[j].name })
			for _, alt := range alts {
				fmt.Fprintf(&unions, "type %s struct { Value %s }\nfunc (%s) skgoParam_%x() {}\n", alt.name, imports.typeExpr(alt.typ), alt.name, key)
			}
		}
		fmt.Fprintf(body, "value%s %s\n", field, result)
		fmt.Fprintf(&accessors, "func (p LayoutParams) %s() %s {skgo.TrackLoadParam(p.event,%q);return p.value%s}\n", field, result, key, field)
	}
	body.WriteString("}\n")
	body.WriteString(unions.String())
	body.WriteString(accessors.String())
	if hasUnion {
		populate.WriteString("switch skgo.LoadRouteIDValue(event) {\n")
		for _, id := range ids {
			fmt.Fprintf(&populate, "case %q:\n", id)
			for _, param := range info.children[id].params {
				if len(alternatives[param.Name]) < 2 {
					continue
				}
				typ := loadParamType(param, info.children[id].matchers)
				fmt.Fprintf(&populate, "if value := skgo.OptionalLoadParamValue[%s](event,%q); value != nil {p.value%s = %s{Value:*value}}\n", imports.typeExpr(typ), param.Name, sharedParamStem(param.Name), sharedAlternativeName(alternatives, param.Name, typ))
			}
		}
		populate.WriteString("}\n")
	}
	writeLoadEvent(body, cfg, "LayoutParams", "Layout", populate.String())
	return nil
}
