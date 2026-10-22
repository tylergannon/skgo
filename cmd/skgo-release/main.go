// Command skgo-release packages the CLI and publishes its Homebrew cask.
// Apple credential handling follows Gimbal's ci-signing.sh and
// sign-notarize-macos-cli.sh, without desktop or Sparkle packaging.
package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const module = "github.com/tylergannon/skgo"

var tagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: skgo-release build TAG SHA | signing setup|cleanup | tap DIR TAG | checksums DIR TAG")
	}
	switch args[0] {
	case "build":
		if len(args) != 3 || !tagPattern.MatchString(args[1]) {
			return fmt.Errorf("build requires a stable tag and source SHA")
		}
		return build(args[1], args[2])
	case "signing":
		if len(args) != 2 {
			return fmt.Errorf("signing requires setup or cleanup")
		}
		if args[1] == "cleanup" {
			return cleanupSigning()
		}
		if args[1] == "setup" {
			return setupSigning()
		}
	case "tap":
		if len(args) != 3 {
			return fmt.Errorf("tap requires checkout directory and tag")
		}
		return updateTap(args[1], args[2])
	case "checksums":
		if len(args) != 3 {
			return fmt.Errorf("checksums requires directory and tag")
		}
		return checksums(args[1], args[2])
	}
	return fmt.Errorf("invalid release command")
}
func command(env []string, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Env = append(os.Environ(), env...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return nil
}
func output(env []string, name string, args ...string) ([]byte, error) {
	c := exec.Command(name, args...)
	c.Env = append(os.Environ(), env...)
	c.Stderr = os.Stderr
	b, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", name, err)
	}
	return b, nil
}
func archiveName(tag, target string) string {
	return "skgo_" + strings.TrimPrefix(tag, "v") + "_" + target + ".tar.gz"
}

func build(tag, sha string) error {
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(sha) {
		return fmt.Errorf("expected full source SHA")
	}
	stage, err := os.MkdirTemp("", "skgo-release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	// Build a released module, rather than a checkout reporting (devel). Isolate
	// this module cache so a retry cannot pick up an earlier use of the same tag.
	// Other dependencies use the normal proxy and sum database.
	env := []string{"GOBIN=" + stage, "GOMODCACHE=" + filepath.Join(stage, "mod"), "GOPRIVATE=" + module, "CGO_ENABLED=1", "GOTOOLCHAIN=go1.27.2", "GOFLAGS=" + os.Getenv("GOFLAGS") + " -modcacherw"}
	// The download response binds the module used by go install to the qualified
	// commit, even if an administrator moved the candidate tag meanwhile.
	data, err := output(env, "go", "mod", "download", "-json", module+"@"+tag)
	if err != nil {
		return err
	}
	var downloaded struct {
		Origin struct{ Hash string }
		Error  string
	}
	if err = json.Unmarshal(data, &downloaded); err != nil {
		return err
	}
	if downloaded.Error != "" || downloaded.Origin.Hash != sha {
		return fmt.Errorf("downloaded release source does not match qualified SHA %s", sha)
	}
	if err = command(env, "go", "install", "-trimpath", module+"/cmd/skgo@"+tag); err != nil {
		return err
	}
	binary := filepath.Join(stage, "skgo")
	info, err := buildinfo.ReadFile(binary)
	if err != nil {
		return err
	}
	if info.Main.Path != module || info.Main.Version != tag || info.GoVersion != "go1.27.2" {
		return fmt.Errorf("packaged CLI has unexpected module/toolchain identity: %v", info)
	}
	if err = command(nil, binary, "buildinfo"); err != nil {
		return err
	}
	emptyPlugins := filepath.Join(stage, "empty-plugins")
	if err = os.Mkdir(emptyPlugins, 0700); err != nil {
		return err
	}
	if err = command([]string{"SKGO_PLUGIN_DIRS=" + emptyPlugins}, binary, "--help"); err != nil {
		return err
	}
	if runtime.GOOS == "darwin" {
		if err = signAndNotarize(binary, stage); err != nil {
			return err
		}
	}
	if err = os.MkdirAll("dist", 0755); err != nil {
		return err
	}
	return writeArchive(filepath.Join("dist", archiveName(tag, runtime.GOOS+"_"+runtime.GOARCH)), map[string]string{"skgo": binary, "LICENSE": "LICENSE", "README.md": "README.md"})
}
func writeArchive(path string, files map[string]string) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { err = firstError(err, f.Close()) }()
	gz := gzip.NewWriter(f)
	defer func() { err = firstError(err, gz.Close()) }()
	tw := tar.NewWriter(gz)
	defer func() { err = firstError(err, tw.Close()) }()
	for _, name := range []string{"skgo", "LICENSE", "README.md"} {
		data, e := os.ReadFile(files[name])
		if e != nil {
			return e
		}
		mode := int64(0644)
		if name == "skgo" {
			mode = 0755
		}
		if e = tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(data)), Typeflag: tar.TypeReg}); e != nil {
			return e
		}
		if _, e = tw.Write(data); e != nil {
			return e
		}
	}
	return nil
}
func firstError(a, b error) error {
	if a != nil {
		return a
	}
	return b
}
func digest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func checksums(dir, tag string) error {
	if !tagPattern.MatchString(tag) {
		return fmt.Errorf("invalid release tag")
	}
	var b strings.Builder
	for _, target := range []string{"darwin_arm64", "linux_amd64", "linux_arm64"} {
		name := archiveName(tag, target)
		sum, err := digest(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s  %s\n", sum, name)
	}
	return os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(b.String()), 0644)
}
func cask(tag, sum string) (string, error) {
	if !tagPattern.MatchString(tag) || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(sum) {
		return "", fmt.Errorf("invalid tag or SHA-256")
	}
	return fmt.Sprintf(`cask "skgo" do
  version "%s"
  sha256 "%s"

  url "https://github.com/tylergannon/skgo/releases/download/v#{version}/skgo_#{version}_darwin_arm64.tar.gz"
  name "SKGO"
  desc "SvelteKit application server and tooling for Go"
  homepage "https://github.com/tylergannon/skgo"

  depends_on arch: :arm64

  binary "skgo"
end
`, strings.TrimPrefix(tag, "v"), sum), nil
}
func updateTap(dir, tag string) error {
	sum, err := digest(filepath.Join("dist", archiveName(tag, "darwin_arm64")))
	if err != nil {
		return err
	}
	ruby, err := cask(tag, sum)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Join(dir, "Casks"), 0755); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "Casks", "skgo.rb"), []byte(ruby), 0644); err != nil {
		return err
	}
	if err = command(nil, "git", "-C", dir, "add", "Casks/skgo.rb"); err != nil {
		return err
	}
	if c := exec.Command("git", "-C", dir, "diff", "--cached", "--quiet"); c.Run() == nil {
		return nil
	}
	if err = command(nil, "git", "-C", dir, "-c", "user.name=github-actions[bot]", "-c", "user.email=41898282+github-actions[bot]@users.noreply.github.com", "commit", "-m", "Update SKGO to "+tag); err != nil {
		return err
	}
	return command(nil, "git", "-C", dir, "push", "origin", "HEAD:main")
}
