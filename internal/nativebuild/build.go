// Package nativebuild builds the shared native shell with application-supplied
// packages, resources and Info.plist properties.
package nativebuild

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/tylergannon/skgo/internal/templates"
	"github.com/tylergannon/skgo/nativeapp"
)

type Options struct {
	Root, Platform, Preset string
	Out                    io.Writer
}

var component = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
var deployment = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

func Build(ctx context.Context, o Options) error {
	if o.Out == nil {
		o.Out = io.Discard
	}
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("native application builds require macOS and Xcode")
	}
	p, _, err := templates.Project(o.Root, "")
	if err != nil {
		return err
	}
	f, err := os.Open(filepath.Join(p.Root, "native/skgo-native.json"))
	if err != nil {
		return err
	}
	var c nativeapp.Config
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	err = dec.Decode(&c)
	f.Close()
	if err != nil {
		return err
	}
	if err := c.Validate(); err != nil {
		return err
	}
	if o.Platform == "" {
		o.Platform = "macos"
	}
	sdk, goos, target, zigTarget, xcodePlatform, minVersion := "macosx", "darwin", "arm64-apple-macos", "aarch64-macos", "macOS", "14.0"
	switch o.Platform {
	case "macos":
	case "iphone":
		sdk, goos, target, zigTarget, xcodePlatform, minVersion = "iphoneos", "ios", "arm64-apple-ios", "aarch64-ios", "iOS", "17.0"
	case "simulator":
		sdk, goos, target, zigTarget, xcodePlatform, minVersion = "iphonesimulator", "ios", "arm64-apple-ios", "aarch64-ios-simulator", "iOS", "17.0"
	default:
		return fmt.Errorf("platform must be macos, iphone or simulator")
	}
	if v := c.DeploymentTargets[o.Platform]; v != "" {
		minVersion = v
	}
	if !deployment.MatchString(minVersion) {
		return fmt.Errorf("invalid deployment target %q", minVersion)
	}
	target += minVersion
	if o.Platform == "simulator" {
		target += "-simulator"
	}
	if o.Platform == "iphone" {
		zigTarget += "." + minVersion
	}
	preset := nativeapp.Preset{}
	if o.Preset != "" {
		if !component.MatchString(o.Preset) {
			return fmt.Errorf("invalid preset name")
		}
		var ok bool
		preset, ok = c.Presets[o.Preset]
		if !ok {
			return fmt.Errorf("unknown native preset %q", o.Preset)
		}
	}
	outputName := o.Platform
	if o.Preset != "" {
		outputName += "-" + o.Preset
	}
	out := filepath.Join(p.Root, "native/build", outputName)
	sdkRoot := filepath.Join(out, ".sdk")
	spec, err := projectSpec(p.Root, p.Name, out, sdkRoot, xcodePlatform, minVersion, c, preset)
	if err != nil {
		return err
	}
	for dir := out; dir != p.Root; dir = filepath.Dir(dir) {
		if st, e := os.Lstat(dir); e == nil && st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("native output contains a symlink: %s", dir)
		}
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	run := func(dir string, env []string, name string, args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		var buf strings.Builder
		cmd.Stdout = io.MultiWriter(&buf, o.Out)
		cmd.Stderr = o.Out
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return []byte(buf.String()), nil
	}
	data, err := run(p.Root, nil, "go", "list", "-m", "-json", "github.com/tylergannon/skgo")
	if err != nil {
		return err
	}
	var module struct{ Dir string }
	if err := json.Unmarshal(data, &module); err != nil || module.Dir == "" {
		return fmt.Errorf("resolve skgo native sources: %v", err)
	}
	if err := copySDK(filepath.Join(module.Dir, "native"), sdkRoot); err != nil {
		return err
	}
	// Generate one shared Go/Swift projection using the native selection. The
	// ordinary scaffold's locals and hook packages remain the integration points.
	args := []string{"tool", "skgo", "generate", "--web", p.Web, "--out", filepath.Join(p.Root, "internal/skgo"), "--locals-package", p.Module + "/internal/app", "--hook-package", p.Module + "/internal/serverhooks"}
	if len(c.SwiftRemotes) > 0 {
		args = append(args, "--swift-out", filepath.Join(p.Root, "native/Generated.swift"))
	}
	for _, remote := range c.SwiftRemotes {
		args = append(args, "--swift-remote", remote)
	}
	if _, err := run(p.Root, nil, "go", args...); err != nil {
		return err
	}
	if _, err := run(p.Web, []string{"ORIGIN=http://127.0.0.1"}, filepath.Join(p.Web, "node_modules/.bin/vp"), "build"); err != nil {
		return err
	}
	if _, err := run(filepath.Join(sdkRoot, "core"), nil, "mise", "-C", filepath.Join(sdkRoot, "core"), "exec", "--", "zig", "build", "library", "-Dtarget="+zigTarget, "-Doptimize=ReleaseFast"); err != nil {
		return err
	}
	clang, err := run(p.Root, nil, "xcrun", "--sdk", sdk, "-f", "clang")
	if err != nil {
		return err
	}
	sdkPath, err := run(p.Root, nil, "xcrun", "--sdk", sdk, "--show-sdk-path")
	if err != nil {
		return err
	}
	archive := filepath.Join(out, "skgo-host.a")
	args = []string{"build", "-buildmode=c-archive", "-o", archive}
	if len(preset.GoTags) > 0 {
		args = append(args, "-tags", strings.Join(preset.GoTags, ","))
	}
	args = append(args, "./native/host")
	env := []string{"CGO_ENABLED=1", "GOOS=" + goos, "GOARCH=arm64", "CC=" + strings.TrimSpace(string(clang)) + " -target " + target + " -isysroot " + strings.TrimSpace(string(sdkPath))}
	if _, err := run(p.Root, env, "go", args...); err != nil {
		return err
	}
	data, err = json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return err
	}
	specPath := filepath.Join(out, "project.json")
	if err := os.WriteFile(specPath, data, 0644); err != nil {
		return err
	}
	if _, err := run(p.Root, nil, "mise", "-C", sdkRoot, "exec", "--", "xcodegen", "generate", "--no-env", "--spec", specPath, "--project", out); err != nil {
		return err
	}
	args = []string{"-project", filepath.Join(out, p.Name+".xcodeproj"), "-scheme", p.Name, "-configuration", "Debug", "-sdk", sdk, "-derivedDataPath", filepath.Join(out, "DerivedData"), "build"}
	if team := os.Getenv("SKGO_DEVELOPMENT_TEAM"); team != "" {
		args = append(args, "CODE_SIGNING_ALLOWED=YES", "CODE_SIGN_STYLE=Automatic", "DEVELOPMENT_TEAM="+team, "CODE_SIGN_IDENTITY=Apple Development", "-allowProvisioningUpdates")
		if device := os.Getenv("SKGO_DEVICE_ID"); device != "" {
			args = append(args, "-destination", "id="+device, "-allowProvisioningDeviceRegistration")
		}
	} else {
		args = append(args, "CODE_SIGNING_ALLOWED=NO")
	}
	if _, err := run(p.Root, nil, "xcodebuild", args...); err != nil {
		return err
	}
	fmt.Fprintln(o.Out, "Native build products:", filepath.Join(out, "DerivedData/Build/Products"))
	return nil
}
func projectSpec(root, name, out, sdk, platform, minVersion string, c nativeapp.Config, p nativeapp.Preset) (map[string]any, error) {
	path := func(value string) (string, error) {
		if !filepath.IsLocal(value) {
			return "", fmt.Errorf("native contribution path must be project-relative: %q", value)
		}
		return filepath.Join(root, value), nil
	}
	packages := map[string]any{"SKGoNative": map[string]any{"path": filepath.Join(sdk, "swift")}}
	deps := []any{map[string]any{"package": "SKGoNative"}}
	for _, pkg := range c.Packages {
		if pkg.Name == "SKGoNative" || pkg.Name == "" {
			return nil, fmt.Errorf("reserved or empty native package name")
		}
		if _, ok := packages[pkg.Name]; ok {
			return nil, fmt.Errorf("duplicate native package %s", pkg.Name)
		}
		v, err := path(pkg.Path)
		if err != nil {
			return nil, err
		}
		packages[pkg.Name] = map[string]any{"path": v}
		for _, product := range pkg.Products {
			deps = append(deps, map[string]any{"package": pkg.Name, "product": product})
		}
	}
	sources := []any{map[string]any{"path": filepath.Join(root, "native/Sources")}}
	if len(c.SwiftRemotes) > 0 {
		sources = append(sources, map[string]any{"path": filepath.Join(root, "native/Generated.swift")})
	}
	for _, s := range c.Sources {
		v, err := path(s)
		if err != nil {
			return nil, err
		}
		sources = append(sources, map[string]any{"path": v})
	}
	resources := c.Resources
	if p.Resources != nil {
		resources = p.Resources
	}
	for _, s := range resources {
		v, err := path(s)
		if err != nil {
			return nil, err
		}
		sources = append(sources, map[string]any{"path": v, "buildPhase": "resources"})
	}
	info := map[string]any{"CFBundleName": name, "CFBundleDisplayName": name, "CFBundleVersion": "1", "CFBundleShortVersionString": "0.1.0", "NSAppTransportSecurity": map[string]any{"NSAllowsLocalNetworking": true}}
	merge := func(m map[string]any) {
		for k, v := range m {
			info[k] = v
		}
	}
	merge(c.Info)
	if platform == "macOS" {
		info["NSPrincipalClass"] = "NSApplication"
		merge(c.MacOSInfo)
	} else {
		info["UILaunchScreen"] = map[string]any{}
		merge(c.IOSInfo)
	}
	merge(p.Info)
	id := c.BundleID
	if p.BundleSuffix != "" {
		id += "." + p.BundleSuffix
	}
	settings := map[string]any{"PRODUCT_BUNDLE_IDENTIFIER": id, "PRODUCT_NAME": name, "SWIFT_VERSION": "6.0", "ARCHS": "arm64", "ONLY_ACTIVE_ARCH": "YES", "SWIFT_OBJC_BRIDGING_HEADER": filepath.Join(root, "native/Bridge.h"), "OTHER_LDFLAGS": []string{filepath.Join(out, "skgo-host.a"), "-lresolv"}, "CODE_SIGNING_ALLOWED": "NO", "ENABLE_APP_SANDBOX": "NO", "GENERATE_INFOPLIST_FILE": "NO"}
	target := map[string]any{"type": "application", "platform": platform, "deploymentTarget": minVersion, "sources": sources, "dependencies": deps, "info": map[string]any{"path": filepath.Join(out, "Info.plist"), "properties": info}, "settings": map[string]any{"base": settings}}
	return map[string]any{"name": name, "packages": packages, "targets": map[string]any{name: target}}, nil
}
func copySDK(source, dest string) error {
	// This location is disposable build output. Replace stale SDK artifacts when
	// the application changes skgo versions; never compile inside the module cache.
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".build" || d.Name() == ".zig-cache" || d.Name() == "zig-out" || d.Name() == "zig-pkg" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dest, rel), 0755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("native SDK contains a symlink: %s", rel)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dest, rel), b, 0644)
	})
}
