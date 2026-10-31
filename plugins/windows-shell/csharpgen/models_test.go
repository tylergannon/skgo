//go:build skgo_qualification

package csharpgen

import (
	"context"
	"fmt"
	tg "github.com/tylergannon/polytype/typegrammar"
	"go/constant"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestGeneratedCSharpModels(t *testing.T) {
	defs, roots := fixture()
	for _, n := range []string{"Wire", "List", "NativeAPI", "Task"} {
		defs = append(defs, tg.Definition{Name: tg.Name{PackagePath: "fixture", Name: n}, Type: &tg.Object{}})
	}
	enumName := tg.Name{PackagePath: "fixture", Name: "AliasEnum"}
	defs = append(defs, tg.Definition{Name: enumName, Type: &tg.Enum{GoType: enumName, Kind: tg.String, Members: []tg.EnumMember{{Name: "AliasEnum", Value: constant.MakeString("one")}, {Name: "Alias", Value: constant.MakeString("one")}, {Name: "Other", Value: constant.MakeString("two")}}}})
	roots = append(roots, &tg.Ref{Target: enumName})
	odd := tg.Name{PackagePath: "fixture", Name: "OddNames"}
	fields := []tg.Field{}
	for i, key := range []string{"OddNames", "GetType", "class", "a-b", "a_b"} {
		fields = append(fields, tg.Field{GoName: fmt.Sprintf("Field%d", i), JSONName: key, Value: &tg.Required{Type: &tg.Scalar{Kind: tg.String}}})
	}
	defs = append(defs, tg.Definition{Name: odd, Type: &tg.Object{Fields: fields}})
	roots = append(roots, &tg.Ref{Target: odd})
	escapes := tg.Name{PackagePath: "fixture", Name: "Escapes"}
	defs = append(defs, tg.Definition{Name: escapes, Type: &tg.Enum{GoType: escapes, Kind: tg.String, Members: []tg.EnumMember{
		{Name: "Control", Value: constant.MakeString("\x01a")},
		{Name: "NUL", Value: constant.MakeString("\x00face")},
		{Name: "Text", Value: constant.MakeString("café 😀\\n")},
		{Name: "Lines", Value: constant.MakeString("\u0085a\u2028b\u2029c\r\n")},
	}}})
	roots = append(roots, &tg.Ref{Target: escapes})

	got, err := Generate(defs, roots, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files := map[string]string{"Generated.cs": string(got.Source), "Probe.csproj": `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net10.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable><TreatWarningsAsErrors>true</TreatWarningsAsErrors></PropertyGroup></Project>`, "Stub.cs": "namespace SKGoNative {}", "Program.cs": `
using GeneratedRemote;
using System.Text.Json.Nodes;
static void Check(bool yes) { if (!yes) throw new Exception("Model assertion failed"); }
var json = """{"Text":"café 😀","Nullable":null,"State":"open","Values":[1,2],"Fixed":[7,9],"Time":"2026-10-08T01:02:03.123456789Z","Choice":{"kind":"note","Text":"native"},"Choices":[{"kind":"count","Value":4}],"Node":{"Name":"root","Next":{"Name":"leaf","Next":null}}}""";
var value = ModelCodec.DecodeRoot0(JsonNode.Parse(json));
Check(value.Text == "café 😀" && !value.Optional.HasValue && !value.Nullable.HasValue);
Check(value.State == State.Open && value.Values.SequenceEqual(new long[]{1,2}) && value.Fixed.SequenceEqual(new byte[]{7,9}));
Check(value.Time == "2026-10-08T01:02:03.123456789Z" && value.Node.Next.Value.Name == "leaf");
Check(value.Choice is Envelope_Choice.Case0 { Value.Text: "native" });
Check(value.Choices[0] is Envelope_Choices.Case1 { Value.Value: 4 });
var encoded = (JsonObject)ModelCodec.EncodeRoot0(value)!;
Check(!encoded.ContainsKey("Optional") && encoded.ContainsKey("Nullable") && encoded["Nullable"] is null);
foreach(var change in new (string, string?)[]{ ("Nullable",null),("Optional","null"),("State","\"other\""),("Fixed","[1]"),("Unknown","1"),("Values","[true]"),("Values","[9007199254740993]"),("Values","[-9007199254740993]"),("Choice","{\"kind\":\"other\"}"),("Text","1") }) {
 var baseline = (JsonObject)JsonNode.Parse(json)!;
 if(change.Item2 is null) baseline.Remove(change.Item1); else baseline[change.Item1]=JsonNode.Parse(change.Item2);
 try { ModelCodec.DecodeRoot0(baseline); throw new Exception("Accepted " + change.Item1); } catch(NativeModelException) {}
}
foreach(var n in new long[]{9007199254740993, -9007199254740993}) {
 try { Wire.EncodeNumber(n,true); throw new Exception("Accepted unsafe integer"); } catch(NativeModelException) {}
}
try { Wire.EncodeNumber(double.NaN,false); throw new Exception("Accepted NaN"); } catch(NativeModelException) {}
Check(ModelCodec.DecodeRoot1(JsonValue.Create("one")) == AliasEnum.Alias && AliasEnum.Alias == AliasEnum.AliasEnum2);
var odd = ModelCodec.DecodeRoot2(JsonNode.Parse("""{"OddNames":"one","GetType":"two","class":"three","a-b":"four","a_b":"five"}"""));
Check(odd.OddNames2 == "one" && odd.GetType2 == "two" && odd.@class == "three" && odd.a_b == "four" && odd.a_b2 == "five");
Check(ModelCodec.EncodeRoot3(Escapes.Control)!.GetValue<string>() == "\u0001a");
Check(ModelCodec.DecodeRoot3(JsonValue.Create("\u0001a")) == Escapes.Control);
Check(ModelCodec.EncodeRoot3(Escapes.NUL)!.GetValue<string>() == "\u0000face");
Check(ModelCodec.DecodeRoot3(JsonValue.Create("\u0000face")) == Escapes.NUL);
Check(ModelCodec.EncodeRoot3(Escapes.Text)!.GetValue<string>() == "café 😀\\n");
Check(ModelCodec.EncodeRoot3(Escapes.Lines)!.GetValue<string>() == "\u0085a\u2028b\u2029c\r\n");
Check(ModelCodec.DecodeRoot3(JsonValue.Create("\u0085a\u2028b\u2029c\r\n")) == Escapes.Lines);
Console.WriteLine("models: omission/null, closed objects, enums including literal control characters, unions, recursive models, arrays, safe numbers PASS");
`}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "dotnet", "run", "--project", filepath.Join(dir, "Probe.csproj"), "--nologo")
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("generated consumer: %v", err)
	}
}
