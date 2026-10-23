package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The test binary stands in for Apple's executables, so rejection/cleanup
// paths exercise the real release command without credentials or shell mocks.
func TestMain(m *testing.M) {
	if os.Getenv("SKGO_FAKE_APPLE") == "1" {
		name := filepath.Base(os.Args[0])
		args := os.Args[1:]
		f, _ := os.OpenFile(os.Getenv("SKGO_FAKE_LOG"), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
		_ = json.NewEncoder(f).Encode(append([]string{name}, args...))
		_ = f.Close()
		if name == "security" && len(args) > 0 && args[0] == "find-identity" {
			_, _ = os.Stdout.WriteString(`1) 123 "Developer ID Application: Fixture (TEAM)"`)
		}
		if name == "security" && len(args) == 3 && args[0] == "list-keychains" {
			_, _ = os.Stdout.WriteString(os.Getenv("SKGO_FAKE_KEYCHAINS"))
		}
		if name == "xcrun" && len(args) > 1 && args[1] == "submit" {
			_, _ = os.Stdout.WriteString(`{"status":"` + os.Getenv("SKGO_FAKE_NOTARY_STATUS") + `","id":"fixture-submission"}`)
		}
		if name == os.Getenv("SKGO_FAKE_FAIL") {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func fakeApple(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"security", "codesign", "ditto", "xcrun", "spctl"} {
		if err := os.Symlink(exe, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	log := filepath.Join(dir, "calls.jsonl")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SKGO_FAKE_APPLE", "1")
	t.Setenv("SKGO_FAKE_LOG", log)
	return log
}
func TestNotaryRejectionBlocksArchive(t *testing.T) {
	log := fakeApple(t)
	t.Setenv("MACOS_SIGNING_IDENTITY", "fixture")
	t.Setenv("MACOS_KEYCHAIN_PATH", "fixture-keychain")
	t.Setenv("MACOS_NOTARY_PROFILE", "fixture-profile")
	t.Setenv("SKGO_FAKE_NOTARY_STATUS", "Invalid")
	dir := t.TempDir()
	binary := filepath.Join(dir, "skgo")
	_ = os.WriteFile(binary, []byte("fixture"), 0755)
	if err := signAndNotarize(binary, dir); err == nil || !strings.Contains(err.Error(), "not accepted") {
		t.Fatalf("notary rejection did not block publication: %v", err)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"notarytool","log","fixture-submission"`) {
		t.Fatalf("missing rejection diagnostics: %s", data)
	}
	if strings.Contains(string(data), `"spctl"`) {
		t.Fatal("assessed rejected binary")
	}
	entitlements, err := os.ReadFile(filepath.Join(dir, "cli.entitlements.plist"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(entitlements), "com.apple.security.cs.disable-library-validation") {
		t.Fatal("Go plugins would be blocked by library validation")
	}
}
func TestAcceptedNotaryCompletesSigning(t *testing.T) {
	log := fakeApple(t)
	t.Setenv("MACOS_SIGNING_IDENTITY", "fixture")
	t.Setenv("MACOS_KEYCHAIN_PATH", "fixture-keychain")
	t.Setenv("MACOS_NOTARY_PROFILE", "fixture-profile")
	t.Setenv("SKGO_FAKE_NOTARY_STATUS", "Accepted")
	dir := t.TempDir()
	if err := signAndNotarize(filepath.Join(dir, "skgo"), dir); err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"--options","runtime"`, `"--keychain","fixture-keychain"`, `"codesign","--verify","--strict"`, `"--wait","--output-format","json"`} {
		if !strings.Contains(string(calls), expected) {
			t.Fatalf("signing protocol missing %s: %s", expected, calls)
		}
	}
}
func TestSigningFailureDeletesCredentials(t *testing.T) {
	if runtime.GOOS != "darwin" { // This branch is itself a contract, not a skip.
		if err := setupSigning(); err == nil {
			t.Fatal("allowed signing setup outside macOS")
		}
		return
	}
	log := fakeApple(t)
	dir := t.TempDir()
	envFile := filepath.Join(dir, "github-env")
	if err := os.WriteFile(envFile, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("RUNNER_TEMP", dir)
	t.Setenv("GITHUB_ENV", envFile)
	for _, name := range []string{"DEVELOPER_ID_CERTIFICATE_P12_BASE64", "APP_STORE_CONNECT_API_KEY_P8_BASE64"} {
		t.Setenv(name, base64.StdEncoding.EncodeToString([]byte("fixture secret")))
	}
	for _, name := range []string{"DEVELOPER_ID_CERTIFICATE_PASSWORD", "APP_STORE_CONNECT_KEY_ID", "APP_STORE_CONNECT_ISSUER_ID"} {
		t.Setenv(name, "fixture")
	}
	t.Setenv("APPLE_TEAM_ID", "TEAM")
	t.Setenv("SKGO_FAKE_KEYCHAINS", "    \"/Users/Fixture User/Library/Keychains/login.keychain-db\"\n    \"/Library/Keychains/System.keychain\"\n")
	t.Setenv("SKGO_FAKE_FAIL", "xcrun")
	if err := setupSigning(); err == nil {
		t.Fatal("credential storage failure ignored")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "skgo-signing-") {
			t.Fatal("private signing files retained after failure")
		}
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), `"delete-keychain"`) {
		t.Fatal("temporary keychain not deleted after failure")
	}
	if !strings.Contains(string(calls), `"/Users/Fixture User/Library/Keychains/login.keychain-db","/Library/Keychains/System.keychain"`) {
		t.Fatal("existing keychain search paths were not preserved")
	}
}
