"""Probe a shared resource module using SKGo's actual builder and loader.

Run from any directory with python3. All fixture sources/binaries are temporary.
This checks packaging/co-loading, not shell behavior or official release pairing.
"""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

repo = Path(__file__).resolve().parents[1]


def run(args, cwd=repo, env=None):
    result = subprocess.run([str(a) for a in args], cwd=cwd, env=env,
                            text=True, stdout=subprocess.PIPE,
                            stderr=subprocess.STDOUT, timeout=180)
    if result.returncode:
        raise RuntimeError(result.stdout)
    return result.stdout


with tempfile.TemporaryDirectory(prefix="skgo-shared-native-probe-") as directory:
    stage = Path(directory).resolve()
    host, builder = stage / "skgo", stage / "builder"
    # A checkout host uses source paths; official trimpath releases use module versions.
    run(["go", "build", "-o", host, "./cmd/skgo"])
    run(["go", "build", "-o", builder, "./cmd/skgo-plugin-build"])
    print("Built local SKGo host and its plugin builder", flush=True)

    shared = stage / "shared"
    shared.mkdir()
    (shared / "go.mod").write_text("module example.com/native-core\n\ngo 1.27.3\n")
    source = repo / "plugins/native-shell/sdk/core"
    for relative in ("build.zig", "build.zig.zon", "src"):
        item = source / relative
        dest = shared / "core" / relative
        dest.parent.mkdir(parents=True, exist_ok=True)
        if item.is_dir():
            shutil.copytree(item, dest)
        else:
            shutil.copyfile(item, dest)
    (shared / "resources.go").write_text('''package nativecore
import ("crypto/sha256"; "embed"; "fmt")
//go:embed core
var Resources embed.FS
func Identity() string {
    data, err := Resources.ReadFile("core/build.zig")
    if err != nil { panic(err) }
    return fmt.Sprintf("%x", sha256.Sum256(data))
}
''')
    expected = hashlib.sha256((source / "build.zig").read_bytes()).hexdigest()
    artifacts = []
    for platform in ("apple", "windows"):
        plugin = stage / platform
        plugin.mkdir()
        (plugin / "go.mod").write_text(f'''module example.com/{platform}-shell-probe

go 1.27.3
require (
 github.com/tylergannon/skgo v0.0.0
 example.com/native-core v1.0.0
)
replace example.com/native-core => {shared}
''')
        (plugin / "main.go").write_text('''package main
import (
 "context"
 "fmt"
 core "example.com/native-core"
 "github.com/tylergannon/skgo/templateapi"
)
var skgoVersion = "dev"
var Version = "v0.1.0"
type provider struct{}
func SKGoPluginV1() templateapi.Plugin { return provider{} }
func (provider) Describe() templateapi.Descriptor {
 return templateapi.Descriptor{ID: "PLATFORM-shell-probe", Version: Version,
 SkgoVersion: skgoVersion, Command: "shared-core=" + core.Identity()}
}
func (provider) RunCommand(_ context.Context, r templateapi.CommandRequest) error {
 _, err := fmt.Fprintln(r.Stdout, core.Identity()); return err
}
func main() {}
'''.replace("PLATFORM", platform))
        artifact = stage / (platform + ".so")
        run([builder, "--host", host, "--source", plugin, "--skgo-source", repo,
             "--out", artifact])
        artifacts.append(str(artifact))
        print(f"Built {platform} plugin with shared core dependency", flush=True)

    # Removing the dependency sources verifies that consumers need only artifacts.
    shutil.rmtree(shared)
    for order in (artifacts, list(reversed(artifacts))):
        env = dict(os.environ, _SKGO_PLUGIN_PROBE_FILES=json.dumps(order))
        for candidate in order:
            report = stage / "probe.json"
            run([host, "_plugin-probe", candidate, report], cwd=stage, env=env)
            result = json.loads(report.read_text())
            assert not result["Diagnostics"], result
            assert result["ID"] in ("apple-shell-probe", "windows-shell-probe"), result
        output = run([host, "plugin", "--help"], cwd=stage, env=env)
        for platform in ("apple", "windows"):
            assert f"{platform}-shell-probe: shared-core={expected}" in output, output
        assert "warning:" not in output.lower(), output
    print("PASS: both plugins co-load in both orders through SKGo's peer probe", flush=True)
    print("PASS: both expose matching embedded Zig resources after source removal", flush=True)
    print("Scope: local macOS host; no official artifact installation or shell changes", flush=True)
