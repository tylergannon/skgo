// Command build generates disposable iOS probe artifacts from tracked source.
// Run from example/: go run ./native/cmd/build -sdk iphonesimulator.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	sdk := flag.String("sdk", "iphonesimulator", "iphoneos or iphonesimulator")
	flag.Parse()
	if *sdk != "iphoneos" && *sdk != "iphonesimulator" {
		log.Fatal("unsupported SDK")
	}
	run := func(name string, args ...string) string {
		cmd := exec.Command(name, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Fatalf("%s: %v\n%s", name, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	clang := run("xcrun", "--sdk", *sdk, "-f", "clang")
	sdkPath := run("xcrun", "--sdk", *sdk, "--show-sdk-path")
	dir, err := filepath.Abs("native/build/" + *sdk)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "host"), 0755); err != nil {
		log.Fatal(err)
	}
	target := "arm64-apple-ios17.0"
	if *sdk == "iphonesimulator" {
		target += "-simulator"
	}
	cmd := exec.Command("go", "build", "-buildmode=c-archive", "-o", filepath.Join(dir, "host/skgo-host.a"), "./native/host")
	cmd.Env = append(os.Environ(), "GOOS=ios", "GOARCH=arm64", "CGO_ENABLED=1", fmt.Sprintf("CC=%s -target %s -isysroot %s", clang, target, sdkPath))
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "project"), 0755); err != nil {
		log.Fatal(err)
	}

	// Find mise's tracked native pins while generating from example source.
	cmd = exec.Command("mise", "-C", "../native", "exec", "--", "xcodegen", "generate", "--spec", "../example/native/project.yml", "--project", filepath.Join(dir, "project"))
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
	fmt.Println(filepath.Join(dir, "project/SKGoNativeProbe.xcodeproj"))
}
