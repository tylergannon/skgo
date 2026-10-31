//go:build skgo_qualification

package windowsapp_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tylergannon/skgo/plugins/windows-shell/windowsapp"
	"github.com/tylergannon/skgo/templateapi"
)

func TestCSharpNavigationPolicy(t *testing.T) {
	root := t.TempDir()
	_, err := windowsapp.Apply(t.Context(), templateapi.Request{Addon: "windows-app", Project: templateapi.Project{Root: root}, Options: map[string]string{"identity": "Example.Policy"}})
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"Policy.csproj": `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net10.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings><Nullable>enable</Nullable><EnableDefaultCompileItems>false</EnableDefaultCompileItems></PropertyGroup><ItemGroup><Compile Include="Program.cs"/><Compile Include="windows/NavigationPolicy.cs"/></ItemGroup></Project>`,
		"Program.cs": `using SKGoShell;
static void Check(bool value, string claim) { if (!value) throw new Exception(claim); }
var p = new NavigationPolicy(new("https://app.example/start"), [new("https://other.example"), new("http://[::1]:8080")], [new("https://app.example")]);
foreach (var url in new[]{"https://APP.example:443/next", "https://other.example/redirected", "http://[::1]:8080/x"}) Check(p.CanNavigate(url), "allowed: " + url);
foreach (var url in new[]{"https://app.example.evil/", "https://evilapp.example/", "http://app.example/", "https://app.example:444/", "https://user@app.example/", "file:///tmp/x", "javascript:alert(1)", "data:text/html,hi", "about:blank", "https://other.example:444", "/relative", "invalid"}) Check(!p.CanNavigate(url), "blocked: " + url);
Check(p.CanMessage("https://app.example/frame", "https://app.example/top"), "trusted message");
Check(!p.CanMessage("https://other.example/frame", "https://app.example/top"), "navigation doesn't grant messages");
Check(!p.CanMessage("https://app.example/frame", "https://other.example/top"), "current page must also be trusted");
var defaults = new NavigationPolicy(new("https://app.example/"));
Check(defaults.CanNavigate("https://app.example/page") && !defaults.CanNavigate("https://other.example/"), "same-origin default");
Check(!defaults.CanMessage("https://app.example/", "https://app.example/"), "bridge disabled by default");
Console.WriteLine("origin normalization, scheme/port/lookalike rejection and independent bridge trust PASS");`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.CommandContext(t.Context(), "dotnet", "run", "--project", "Policy.csproj")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("navigation consumer: %v\n%s", err, out)
	} else {
		t.Log(string(out))
	}
}
