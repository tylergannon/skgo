package swiftgen

import (
	"go/constant"
	"go/token"
	"strings"
	"testing"

	tg "github.com/tylergannon/polytype/typegrammar"
)

func fixture() (tg.Definitions, []tg.Type) {
	named := func(name string) tg.Name { return tg.Name{PackagePath: "fixture/models", Name: name} }
	ref := func(name string) tg.Type { return &tg.Ref{Target: named(name)} }
	field := func(name string, v tg.FieldValue) tg.Field { return tg.Field{GoName: name, JSONName: name, Value: v} }
	str := &tg.Scalar{Kind: tg.String}
	num := &tg.Scalar{Kind: tg.Int64}
	choice := &tg.Union{Interface: named("Choice"), Discriminator: "kind", Variants: []tg.Variant{{Implementation: named("Note"), Tag: "note"}, {Implementation: named("Count"), Tag: "count", Pointer: true}}}
	defs := tg.Definitions{
		{Name: named("State"), Type: &tg.Enum{GoType: named("State"), Kind: tg.String, Members: []tg.EnumMember{{Name: "Open", Value: constant.MakeString("open")}, {Name: "Closed", Value: constant.MakeString("closed")}}}},
		{Name: named("Note"), Type: &tg.Object{Fields: []tg.Field{field("Text", &tg.Required{Type: str})}}},
		{Name: named("Count"), Type: &tg.Object{Fields: []tg.Field{field("Value", &tg.Required{Type: num})}}},
		{Name: named("Node"), Type: &tg.Object{Fields: []tg.Field{field("Name", &tg.Required{Type: str}), field("Next", &tg.Nullable{Type: &tg.Pointer{Element: ref("Node")}})}}},
		{Name: named("Envelope"), Type: &tg.Object{Fields: []tg.Field{
			field("Text", &tg.Required{Type: str}), field("Optional", &tg.Optional{Type: str}), field("Nullable", &tg.Nullable{Type: str}),
			field("State", &tg.Required{Type: ref("State")}), field("Values", &tg.Required{Type: &tg.Slice{Element: num}}),
			field("Fixed", &tg.Required{Type: &tg.Array{Length: 2, Element: &tg.Scalar{Kind: tg.Uint8}}}), field("Time", &tg.Required{Type: &tg.Time{}}),
			field("Choice", choice), field("OptionalChoice", &tg.OptionalUnion{Union: *choice}), field("Choices", &tg.UnionSlice{Union: *choice}), field("Node", &tg.Required{Type: ref("Node")}),
		}}},
	}
	return defs, []tg.Type{ref("Envelope")}
}
func TestProjectionRejectsSuppliedShapesAtTheirSource(t *testing.T) {
	name := tg.Name{PackagePath: "fixture", Name: "Owner"}
	defs := tg.Definitions{{Name: name, Type: &tg.Object{Fields: []tg.Field{{GoName: "Value", JSONName: "value", Source: token.Position{Filename: "models.go", Line: 17}, Value: &tg.Provided{Ref: "external.json"}}}}}}
	_, err := Generate(defs, []tg.Type{&tg.Ref{Target: name}}, nil)
	if err == nil || !strings.Contains(err.Error(), "models.go:17") || !strings.Contains(err.Error(), "value") {
		t.Fatalf("error = %v", err)
	}
}
func TestSwiftRemoteAdmission(t *testing.T) {
	defs, roots := fixture()
	for _, calls := range [][]Call{
		{{Name: "form", ID: "hash/form", Kind: "form", Input: 0, Output: 0}},
		{{Name: "query", ID: "hash/query", Kind: "query", Input: -1, Output: 4}},
		{{Name: "same", ID: "hash/a", Kind: "query", Input: -1, Output: 0}, {Name: "same", ID: "hash/b", Kind: "command", Input: 0, Output: 0}},
	} {
		if _, err := Generate(defs, roots, calls); err == nil {
			t.Fatalf("admitted %#v", calls)
		}
	}
}

func TestWideEnumIsRejectedAtItsFieldSource(t *testing.T) {
	name := tg.Name{PackagePath: "fixture", Name: "Owner"}
	e := &tg.Enum{GoType: tg.Name{PackagePath: "fixture", Name: "Counter"}, Kind: tg.Uint64, Members: []tg.EnumMember{{Name: "Wide", Value: constant.MakeUint64(9007199254740993)}}}
	defs := tg.Definitions{{Name: name, Type: &tg.Object{Fields: []tg.Field{{GoName: "Counter", JSONName: "counter", Source: token.Position{Filename: "model.go", Line: 23}, Value: &tg.Required{Type: e}}}}}}
	_, err := Generate(defs, []tg.Type{&tg.Ref{Target: name}}, nil)
	if err == nil || !strings.Contains(err.Error(), "model.go:23") || !strings.Contains(err.Error(), "Counter.Wide") {
		t.Fatalf("error=%v", err)
	}
}
