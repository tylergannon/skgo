package main

import (
	"crypto/rand"
	"encoding/base64"
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

func required(names ...string) error {
	for _, name := range names {
		if os.Getenv(name) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	return nil
}
func githubEnv(values map[string]string) error {
	f, err := os.OpenFile(os.Getenv("GITHUB_ENV"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	for key, value := range values {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("invalid environment value for %s", key)
		}
		if _, err = fmt.Fprintf(f, "%s=%s\n", key, value); err != nil {
			return err
		}
	}
	return nil
}
func quiet(name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdout = io.Discard
	c.Stderr = io.Discard
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s credential operation failed: %w", name, err)
	}
	return nil
}
func setupSigning() (err error) {
	if runtime.GOOS != "darwin" || os.Getenv("GITHUB_ACTIONS") != "true" {
		return fmt.Errorf("signing setup requires a macOS Actions job")
	}
	if err = required("RUNNER_TEMP", "GITHUB_ENV", "DEVELOPER_ID_CERTIFICATE_P12_BASE64", "DEVELOPER_ID_CERTIFICATE_PASSWORD", "APPLE_TEAM_ID", "APP_STORE_CONNECT_KEY_ID", "APP_STORE_CONNECT_ISSUER_ID", "APP_STORE_CONNECT_API_KEY_P8_BASE64"); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(os.Getenv("RUNNER_TEMP"), "skgo-signing-")
	if err != nil {
		return err
	}
	keychain := filepath.Join(dir, "release.keychain-db")
	defer func() {
		if err != nil {
			_ = quiet("security", "delete-keychain", keychain)
			_ = os.RemoveAll(dir)
		}
	}()
	if err = githubEnv(map[string]string{"SKGO_CI_SIGNING_DIR": dir, "MACOS_KEYCHAIN_PATH": keychain}); err != nil {
		return err
	}
	p12 := filepath.Join(dir, "developer-id.p12")
	p8 := filepath.Join(dir, "notary.p8")
	for path, name := range map[string]string{p12: "DEVELOPER_ID_CERTIFICATE_P12_BASE64", p8: "APP_STORE_CONNECT_API_KEY_P8_BASE64"} {
		data, e := base64.StdEncoding.DecodeString(os.Getenv(name))
		if e != nil {
			return fmt.Errorf("invalid base64 for %s", name)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			return err
		}
	}
	passwordBytes := make([]byte, 32)
	if _, err = rand.Read(passwordBytes); err != nil {
		return err
	}
	password := hex.EncodeToString(passwordBytes)
	for _, args := range [][]string{
		{"create-keychain", "-p", password, keychain},
		{"set-keychain-settings", "-lut", "21600", keychain},
		{"unlock-keychain", "-p", password, keychain},
		{"import", p12, "-k", keychain, "-P", os.Getenv("DEVELOPER_ID_CERTIFICATE_PASSWORD"), "-T", "/usr/bin/codesign", "-T", "/usr/bin/security"},
		{"set-key-partition-list", "-S", "apple-tool:,apple:", "-s", "-k", password, keychain},
	} {
		if err = quiet("security", args...); err != nil {
			return err
		}
	}
	identities, err := output(nil, "security", "find-identity", "-v", "-p", "codesigning", keychain)
	if err != nil {
		return err
	}
	matches := regexp.MustCompile(`"(Developer ID Application: [^"\n]+)"`).FindAllSubmatch(identities, -1)
	if len(matches) != 1 || !strings.HasSuffix(string(matches[0][1]), "("+os.Getenv("APPLE_TEAM_ID")+")") {
		return fmt.Errorf("expected exactly one valid Developer ID Application identity for the configured Apple team")
	}
	const profile = "skgo-release-notary"
	if err = quiet("xcrun", "notarytool", "store-credentials", profile, "--key", p8, "--key-id", os.Getenv("APP_STORE_CONNECT_KEY_ID"), "--issuer", os.Getenv("APP_STORE_CONNECT_ISSUER_ID"), "--keychain", keychain); err != nil {
		return err
	}
	if err = os.Remove(p12); err != nil {
		return err
	}
	if err = os.Remove(p8); err != nil {
		return err
	}
	return githubEnv(map[string]string{"MACOS_SIGNING_IDENTITY": string(matches[0][1]), "MACOS_NOTARY_PROFILE": profile})
}
func cleanupSigning() error {
	dir := os.Getenv("SKGO_CI_SIGNING_DIR")
	if dir == "" {
		return nil
	}
	// Only delete the temporary directory created by this command.
	if !strings.HasPrefix(filepath.Base(dir), "skgo-signing-") || filepath.Dir(dir) != filepath.Clean(os.Getenv("RUNNER_TEMP")) {
		return fmt.Errorf("refusing unexpected signing directory")
	}
	keychain := filepath.Join(dir, "release.keychain-db")
	if _, err := os.Stat(keychain); os.IsNotExist(err) {
		return os.RemoveAll(dir)
	}
	err := quiet("security", "delete-keychain", keychain)
	return firstError(err, os.RemoveAll(dir))
}
func signAndNotarize(binary, stage string) error {
	if err := required("MACOS_SIGNING_IDENTITY", "MACOS_NOTARY_PROFILE", "MACOS_KEYCHAIN_PATH"); err != nil {
		return err
	}
	// SKGO's CLI supports separately compiled Go template plugins, just as
	// Gimbal's CLI does. Production application servers remain cgo-free.
	entitlements := filepath.Join(stage, "cli.entitlements.plist")
	if err := os.WriteFile(entitlements, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>com.apple.security.cs.disable-library-validation</key><true/></dict></plist>
`), 0600); err != nil {
		return err
	}
	if err := command(nil, "codesign", "--force", "--timestamp", "--options", "runtime", "--keychain", os.Getenv("MACOS_KEYCHAIN_PATH"), "--entitlements", entitlements, "--sign", os.Getenv("MACOS_SIGNING_IDENTITY"), binary); err != nil {
		return err
	}
	if err := command(nil, "codesign", "--verify", "--strict", "--verbose=2", binary); err != nil {
		return err
	}
	zip := filepath.Join(stage, "skgo-cli.zip")
	if err := command(nil, "ditto", "-c", "-k", "--sequesterRsrc", "--keepParent", binary, zip); err != nil {
		return err
	}
	args := []string{"notarytool", "submit", zip, "--keychain-profile", os.Getenv("MACOS_NOTARY_PROFILE"), "--keychain", os.Getenv("MACOS_KEYCHAIN_PATH"), "--wait", "--output-format", "json"}
	response, submitErr := output(nil, "xcrun", args...)
	var result struct {
		Status string `json:"status"`
		ID     string `json:"id"`
	}
	if err := json.Unmarshal(response, &result); err != nil {
		return firstError(submitErr, err)
	}
	if submitErr != nil || result.Status != "Accepted" {
		if result.ID != "" {
			_ = command(nil, "xcrun", "notarytool", "log", result.ID, "--keychain-profile", os.Getenv("MACOS_NOTARY_PROFILE"), "--keychain", os.Getenv("MACOS_KEYCHAIN_PATH"))
		}
		return fmt.Errorf("CLI notarization was not accepted: %s (%v)", result.Status, submitErr)
	}
	fmt.Printf("Apple accepted CLI notarization %s\n", result.ID)
	// CLI executables cannot staple tickets; Gatekeeper performs an online
	// lookup on first use. Preserve the accepted executable in the tarball.
	return nil
}
