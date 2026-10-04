// Command skgo-e2e runs the regular browser projects and source-edit project
// in separate, ordered Playwright invocations. Source-edit mutates the live
// example tree, so it cannot overlap the regular scenarios.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	jsonOutputEnv     = "PLAYWRIGHT_JSON_OUTPUT_NAME"
	jsonOutputDirEnv  = "PLAYWRIGHT_JSON_OUTPUT_DIR"
	jsonOutputFileEnv = "PLAYWRIGHT_JSON_OUTPUT_FILE"
	blobOutputEnv     = "PLAYWRIGHT_BLOB_OUTPUT_NAME"
	blobOutputDirEnv  = "PLAYWRIGHT_BLOB_OUTPUT_DIR"
	blobOutputFileEnv = "PLAYWRIGHT_BLOB_OUTPUT_FILE"
)

var blobOutputEnvs = []string{blobOutputEnv, blobOutputDirEnv, blobOutputFileEnv}
var jsonOutputEnvs = []string{jsonOutputEnv, jsonOutputDirEnv, jsonOutputFileEnv}

type stage struct {
	name     string
	projects []string
}

var stages = []stage{
	{name: "regular", projects: []string{"chromium", "noscript"}},
	{name: "source-edit", projects: []string{"source-edit"}},
}

type commandRunner func(context.Context, string, []string, []string) error

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := run(ctx, os.Args[1:], os.Environ(), execPlaywright)
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	if errors.Is(err, context.Canceled) && ctx.Err() != nil {
		os.Exit(130)
	}
	os.Exit(1)
}

func run(ctx context.Context, args, environ []string, invoke commandRunner) error {
	args, err := withoutProjectSelection(args)
	if err != nil {
		return err
	}

	workingDir, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolve Playwright working directory: %w", err)
	}
	baseRun := envValue(environ, "SKGO_E2E_RUN")
	if baseRun == "" {
		baseRun = "run"
	}

	jsonPath, nativeJSONOutput, err := resolveJSONOutputPath(args, environ)
	if err != nil {
		return err
	}
	selectedReporter, hasReporter := reporterSelection(args)
	if nativeJSONOutput {
		if jsonPath != "" {
			if err := os.MkdirAll(filepath.Dir(jsonPath), 0o755); err != nil {
				return fmt.Errorf("create JSON report directory: %w", err)
			}
			// A prior report at a known native environment destination must not
			// masquerade as the current run if report generation fails.
			if err := os.Remove(jsonPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove prior Playwright JSON report: %w", err)
			}
		}
		if !hasReporter || !hasJSONReporter(args) {
			// Supplying a native JSON destination is an explicit request for the
			// JSON reporter, even when the config normally selects another one.
			selectedReporter = "json"
			hasReporter = true
		}
	}

	scratchDir, err := os.MkdirTemp("", "skgo-e2e-report-scratch-")
	if err != nil {
		return fmt.Errorf("create temporary Playwright report directory: %w", err)
	}
	defer os.RemoveAll(scratchDir)

	var failures []error
	blobPaths := make([]string, 0, len(stages))
	for _, current := range stages {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		blobDir := filepath.Join(scratchDir, current.name)
		if err := os.MkdirAll(blobDir, 0o755); err != nil {
			return errors.Join(append(failures, fmt.Errorf("create %s Playwright blob directory: %w", current.name, err))...)
		}
		blobPath := filepath.Join(blobDir, current.name+".zip")
		blobPaths = append(blobPaths, blobPath)

		stageEnv := setEnv(environ, "SKGO_E2E_RUN", baseRun+"-"+current.name)
		stageEnv = setEnv(stageEnv, "SKGO_E2E_STAGE", current.name)
		for _, key := range jsonOutputEnvs {
			stageEnv = unsetEnv(stageEnv, key)
		}
		for _, key := range blobOutputEnvs {
			stageEnv = unsetEnv(stageEnv, key)
		}
		stageEnv = setEnv(stageEnv, blobOutputDirEnv, blobDir)
		stageEnv = setEnv(stageEnv, blobOutputFileEnv, blobPath)

		commandArgs := []string{"test"}
		for _, project := range current.projects {
			commandArgs = append(commandArgs, "--project="+project)
		}
		stageArgs := testArgsWithReporter(args, "blob")
		commandArgs = append(commandArgs, stageArgs...)
		if err := invoke(ctx, "./node_modules/.bin/playwright", commandArgs, stageEnv); err != nil {
			if ctx.Err() != nil {
				return errors.Join(append(failures, ctx.Err())...)
			}
			failures = append(failures, fmt.Errorf("%s Playwright stage failed: %w", current.name, err))
		}
	}

	retainedDir, err := retainBlobReports(blobPaths)
	if err != nil {
		failures = append(failures, fmt.Errorf("retain Playwright blob reports: %w", err))
		return errors.Join(failures...)
	}

	configArgs := mergeConfigArgs(args, workingDir)
	mergeArgs := []string{"merge-reports", retainedDir}
	mergeArgs = append(mergeArgs, configArgs...)
	if hasReporter {
		mergeArgs = append(mergeArgs, "--reporter", selectedReporter)
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(append(failures, err)...)
	}
	if err := invoke(ctx, "./node_modules/.bin/playwright", mergeArgs, environ); err != nil {
		if ctx.Err() != nil {
			failures = append(failures, ctx.Err())
		} else {
			failures = append(failures, fmt.Errorf("merge Playwright reports failed: %w", err))
		}
	}
	return errors.Join(failures...)
}

func execPlaywright(ctx context.Context, name string, args, environ []string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = environ
	command.Stdout = os.Stdout
	if len(args) != 0 && args[0] == "test" {
		// Keep stage progress off stdout so an explicit JSON reporter on the
		// final merge can emit one parseable JSON document there.
		command.Stdout = os.Stderr
	}
	command.Stderr = os.Stderr
	return command.Run()
}

func envValue(environ []string, key string) string {
	for _, entry := range environ {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name == key {
			return value
		}
	}
	return ""
}

func setEnv(environ []string, key, value string) []string {
	result := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || name != key {
			result = append(result, entry)
		}
	}
	return append(result, key+"="+value)
}

func unsetEnv(environ []string, key string) []string {
	result := make([]string, 0, len(environ))
	for _, entry := range environ {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || name != key {
			result = append(result, entry)
		}
	}
	return result
}

// resolveJSONOutputPath mirrors Playwright's resolveOutputFile environment
// precedence for the JSON reporter. Relative FILE and DIR values are resolved
// from the process working directory; relative NAME values are resolved from
// DIR or, when no DIR is supplied, the resolved Playwright config directory.
func resolveJSONOutputPath(args, environ []string) (string, bool, error) {
	workingDir, err := filepath.Abs(".")
	if err != nil {
		return "", false, fmt.Errorf("resolve Playwright working directory: %w", err)
	}
	if outputFile := envValue(environ, jsonOutputFileEnv); outputFile != "" {
		path, err := resolveFrom(workingDir, outputFile)
		if err != nil {
			return "", false, fmt.Errorf("resolve Playwright JSON output file: %w", err)
		}
		return path, true, nil
	}
	outputName := envValue(environ, jsonOutputEnv)
	if outputName == "" {
		return "", false, nil
	}
	outputDir := envValue(environ, jsonOutputDirEnv)
	if outputDir == "" {
		outputDir, err = playwrightConfigDir(args, workingDir)
		if err != nil {
			return "", false, fmt.Errorf("resolve Playwright config directory for JSON output: %w", err)
		}
	} else {
		outputDir, err = resolveFrom(workingDir, outputDir)
		if err != nil {
			return "", false, fmt.Errorf("resolve Playwright JSON output directory: %w", err)
		}
	}
	path, err := resolveFrom(outputDir, outputName)
	if err != nil {
		return "", false, fmt.Errorf("resolve Playwright JSON output name: %w", err)
	}
	return path, true, nil
}

// resolveFrom matches Node's path.resolve(base, value) for the current host:
// an absolute value overrides base, while a relative value is joined to it.
func resolveFrom(base, value string) (string, error) {
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Abs(filepath.Join(base, value))
}

// Playwright's configDir is the config file's directory, or the selected
// config directory when no config file is present there. Without --config,
// Playwright starts discovery in the process working directory.
func playwrightConfigDir(args []string, workingDir string) (string, error) {
	configLocation, configured := configLocation(args)
	if !configured || configLocation == "" {
		return workingDir, nil
	}
	resolved, err := resolveFrom(workingDir, configLocation)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(resolved); err == nil && info.IsDir() {
		return resolved, nil
	}
	return filepath.Dir(resolved), nil
}

func configLocation(args []string) (string, bool) {
	location := ""
	found := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if arg == "-c" || arg == "--config" {
			found = true
			if i+1 < len(args) {
				i++
				location = args[i]
			}
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--config="); ok {
			location, found = value, true
		} else if strings.HasPrefix(arg, "-c") && len(arg) > len("-c") {
			// Playwright accepts a value attached directly to -c. An equals
			// sign in -c=<path> is part of that value; only the long option uses
			// an equals delimiter.
			location, found = strings.TrimPrefix(arg, "-c"), true
		}
	}
	return location, found
}

func mergeConfigArgs(args []string, workingDir string) []string {
	location, configured := configLocation(args)
	if !configured || location == "" {
		// merge-reports otherwise loads an empty config and loses the config
		// Playwright would discover from the test command's working directory.
		location = workingDir
	}
	return []string{"--config", location}
}

func reporterSelection(args []string) (string, bool) {
	reporter := ""
	found := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if arg == "--reporter" || arg == "-r" {
			found = true
			reporter = ""
			if i+1 < len(args) {
				i++
				reporter = args[i]
			}
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--reporter="); ok {
			reporter, found = value, true
		} else if value, ok := strings.CutPrefix(arg, "-r="); ok {
			reporter, found = value, true
		} else if strings.HasPrefix(arg, "-r") && len(arg) > len("-r") {
			reporter, found = strings.TrimPrefix(arg, "-r"), true
		}
	}
	return reporter, found
}

func withoutReporterSelection(args []string) []string {
	result := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			result = append(result, args[i:]...)
			break
		}
		if arg == "--reporter" || arg == "-r" {
			if i+1 < len(args) {
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "--reporter=") || strings.HasPrefix(arg, "-r=") || strings.HasPrefix(arg, "-r") && len(arg) > len("-r") {
			continue
		}
		result = append(result, arg)
	}
	return result
}

func hasJSONReporter(args []string) bool {
	reporter, found := reporterSelection(args)
	if !found {
		return false
	}
	for _, name := range strings.Split(reporter, ",") {
		if strings.TrimSpace(name) == "json" {
			return true
		}
	}
	return false
}

func testArgsWithReporter(args []string, reporter string) []string {
	result := withoutReporterSelection(args)
	owned := "--reporter=" + reporter
	for i, arg := range result {
		if arg == "--" {
			result = append(result[:i], append([]string{owned}, result[i:]...)...)
			return result
		}
	}
	return append(result, owned)
}

func retainBlobReports(paths []string) (string, error) {
	if len(paths) != len(stages) {
		return "", fmt.Errorf("got %d Playwright blob reports, want %d", len(paths), len(stages))
	}
	if err := os.MkdirAll("test-results", 0o755); err != nil {
		return "", fmt.Errorf("create test-results directory: %w", err)
	}
	retainedDir, err := os.MkdirTemp("test-results", "skgo-e2e-blobs-")
	if err != nil {
		return "", fmt.Errorf("create retained blob directory: %w", err)
	}
	retainedDir, err = filepath.Abs(retainedDir)
	if err != nil {
		return "", fmt.Errorf("resolve retained blob directory: %w", err)
	}
	for index, source := range paths {
		destination := filepath.Join(retainedDir, stages[index].name+".zip")
		if err := copyFile(destination, source); err != nil {
			return retainedDir, fmt.Errorf("copy %s blob report: %w", stages[index].name, err)
		}
	}
	return retainedDir, nil
}

func copyFile(destination, source string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(destination)
		return closeErr
	}
	return nil
}

func withoutProjectSelection(args []string) ([]string, error) {
	if len(args) != 0 && args[0] == "--" {
		// A leading separator used to forward Playwright options through the
		// wrapper is not a Playwright option; remove it before invocation.
		args = args[1:]
	}
	result := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--project" || arg == "-p" {
			if i+1 == len(args) {
				return nil, fmt.Errorf("%s needs a project name", arg)
			}
			i++
			continue
		}
		if strings.HasPrefix(arg, "--project=") || strings.HasPrefix(arg, "-p=") {
			continue
		}
		result = append(result, arg)
	}
	return result, nil
}
