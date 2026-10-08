//go:build darwin

package swiftgen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tg "github.com/tylergannon/polytype/typegrammar"
)

func TestGeneratedModelsCompileAndEnforceTheWireShape(t *testing.T) {
	defs, roots := fixture()
	for _, name := range []string{"Error", "Sendable", "FixedWidthInteger", "BinaryInteger", "BinaryFloatingPoint", "CFGetTypeID", "CFBooleanGetTypeID"} {
		defs = append(defs, tg.Definition{Name: tg.Name{PackagePath: "fixture/models", Name: name}, Type: &tg.Object{}})
	}
	odd := tg.Name{PackagePath: "fixture/models", Name: "OddNames"}
	fields := []tg.Field{}
	for i, key := range []string{"_", "-", "self", "class", "a-b", "a_b"} {
		fields = append(fields, tg.Field{GoName: fmt.Sprintf("Field%d", i), JSONName: key, Value: &tg.Required{Type: &tg.Scalar{Kind: tg.String}}})
	}
	defs = append(defs, tg.Definition{Name: odd, Type: &tg.Object{Fields: fields}})
	roots = append(roots, &tg.Ref{Target: odd})
	result, err := Generate(defs, roots, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Error", "Sendable", "FixedWidthInteger", "BinaryInteger", "BinaryFloatingPoint", "CFGetTypeID", "CFBooleanGetTypeID"} {
		if got := result.Names[tg.Name{PackagePath: "fixture/models", Name: name}]; got != name+"2" {
			t.Fatalf("%s projected as %s", name, got)
		}
	}
	probe := `
@main struct Probe {
static func main() throws {
    let json = #"{"Text":"café 😀","Nullable":null,"State":"open","Values":[1,2],"Fixed":[7,9],"Time":"2026-10-08T01:02:03.123456789Z","Choice":{"kind":"note","Text":"native"},"Choices":[{"kind":"count","Value":4}],"Node":{"Name":"root","Next":{"Name":"leaf","Next":null}}}"#
    let value = try _skgoDecodeRoot_0(JSONSerialization.jsonObject(with: Data(json.utf8)), 0)
    precondition(value.Text == "café 😀" && value.Optional == nil && value.Nullable == nil)
    precondition(value.State == .Open && value.Values == [1,2] && value.Fixed == [7,9])
    precondition(value.Time == "2026-10-08T01:02:03.123456789Z" && value.Node.Next?.Name == "leaf")
    if case .Note(let note) = value.Choice { precondition(note.Text == "native") } else { fatalError("wrong union") }
    if case .Count(let count) = value.Choices[0] { precondition(count.Value == 4) } else { fatalError("wrong union slice") }
    let encoded = try _skgoEncodeRoot_0(value, 0) as! [String: Any]
    precondition(encoded["Optional"] == nil && encoded["Nullable"] is NSNull)
    precondition((encoded["Choice"] as! [String: Any])["kind"] as! String == "note")
    let baseline = try JSONSerialization.jsonObject(with: Data(json.utf8)) as! [String:Any]
    let changes: [(String,Any)] = [("Nullable", "DELETE"),("Optional",NSNull()),("State","other"),("Fixed",[1]),("Unknown",1),("Values",[true]),("Values",[9007199254740993]),("Values",[-9007199254740993]),("Choice",["kind":"other"]),("Text",1)]
    for (key, change) in changes {
        var object = baseline
        if key == "Nullable" { object.removeValue(forKey:key) } else { object[key] = change }
        do { _ = try _skgoDecodeRoot_0(object, 0); fatalError("accepted \(key)") } catch is NativeModelError {}
    }
    do { _ = try _SKGoWire.encodeInteger(UInt64.max); fatalError("accepted wide integer") } catch is NativeModelError {}
    do { _ = try _SKGoWire.encodeInteger(Int64(9007199254740993)); fatalError("accepted unsafe positive integer") } catch is NativeModelError {}
    do { _ = try _SKGoWire.encodeInteger(Int64(-9007199254740993)); fatalError("accepted unsafe negative integer") } catch is NativeModelError {}
    let odd = try _skgoDecodeRoot_1(JSONSerialization.jsonObject(with:Data(#"{"_":"one","-":"two","self":"three","class":"four","a-b":"five","a_b":"six"}"#.utf8)),0)
    precondition(odd.field == "one" && odd.field2 == "two" && odd.self2 == "three" && odd.class == "four" && odd.a_b == "five" && odd.a_b2 == "six")
    let oddWire = try _skgoEncodeRoot_1(odd,0) as! [String:Any]
    precondition(oddWire["_"] as! String == "one" && oddWire["self"] as! String == "three" && oddWire["a-b"] as! String == "five")
    print("models and rejected shapes verified")
}
}
`
	dir := t.TempDir()
	source := filepath.Join(dir, "Generated.swift")
	binary := filepath.Join(dir, "probe")
	if err := os.WriteFile(source, append(result.Source, []byte(probe)...), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "swiftc", "-swift-version", "6", "-parse-as-library", source, "-o", binary)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile generated consumer: %v\n%s\n%s", err, out, result.Source)
	}
	out, err := exec.CommandContext(t.Context(), binary).CombinedOutput()
	if err != nil {
		t.Fatalf("consumer: %v\n%s", err, out)
	}
	if string(out) != "models and rejected shapes verified\n" {
		t.Fatalf("output=%q", out)
	}
}
