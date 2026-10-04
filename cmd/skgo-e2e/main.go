// Command skgo-e2e runs the regular browser projects and source-edit project
// in separate, ordered Playwright invocations. Source-edit mutates the live
// example tree, so it cannot overlap the regular scenarios.
package main

import (
	"context"
	"encoding/json"
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
)

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

	baseRun := envValue(environ, "SKGO_E2E_RUN")
	if baseRun == "" {
		baseRun = "run"
	}
	jsonPath, nativeJSONOutput, err := resolveJSONOutputPath(args, environ)
	if err != nil {
		return err
	}
	jsonToStdout := !nativeJSONOutput && hasJSONReporter(args)
	if nativeJSONOutput && !hasJSONReporter(args) {
		// Supplying Playwright's JSON output path is an explicit request for the
		// JSON reporter; the config's normal list/HTML reporters otherwise
		// ignore the PLAYWRIGHT_JSON_OUTPUT_* environment variables.
		args = append(args, "--reporter=json")
	}
	if jsonToStdout {
		jsonPath = filepath.Join("playwright-report", fmt.Sprintf("combined-%d.json", os.Getpid()))
		jsonPath, err = filepath.Abs(jsonPath)
		if err != nil {
			return fmt.Errorf("resolve combined Playwright JSON report path: %w", err)
		}
	}
	if jsonPath != "" {
		if err := os.MkdirAll(filepath.Dir(jsonPath), 0o755); err != nil {
			return fmt.Errorf("create JSON report directory: %w", err)
		}
		// A prior report at the destination must never masquerade as this run's
		// result when one of the stages fails to write JSON.
		if err := os.Remove(jsonPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove prior Playwright JSON report: %w", err)
		}
	}

	var stageReports []string
	var failures []error
	for _, current := range stages {
		if err := ctx.Err(); err != nil {
			return err
		}
		stageEnv := setEnv(environ, "SKGO_E2E_RUN", baseRun+"-"+current.name)
		stageEnv = setEnv(stageEnv, "SKGO_E2E_STAGE", current.name)
		if jsonPath != "" {
			path := stageReportPath(jsonPath, current.name, os.Getpid())
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove prior %s Playwright report: %w", current.name, err)
			}
			stageReports = append(stageReports, path)
			stageEnv = unsetEnv(stageEnv, jsonOutputDirEnv)
			stageEnv = unsetEnv(stageEnv, jsonOutputEnv)
			stageEnv = setEnv(stageEnv, jsonOutputFileEnv, path)
		}

		commandArgs := []string{"test"}
		for _, project := range current.projects {
			commandArgs = append(commandArgs, "--project="+project)
		}
		commandArgs = append(commandArgs, args...)
		if err := invoke(ctx, "./node_modules/.bin/playwright", commandArgs, stageEnv); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			failures = append(failures, fmt.Errorf("%s Playwright stage failed: %w", current.name, err))
		}
	}

	if len(stageReports) != 0 {
		if err := combineReports(jsonPath, stageReports); err != nil {
			failures = append(failures, fmt.Errorf("combine Playwright JSON reports: %w", err))
		} else {
			for _, report := range stageReports {
				_ = os.Remove(report)
			}
			if jsonToStdout {
				if err := copyReport(os.Stdout, jsonPath); err != nil {
					failures = append(failures, fmt.Errorf("write combined Playwright JSON report: %w", err))
				}
			}
		}
	}
	return errors.Join(failures...)
}

func execPlaywright(ctx context.Context, name string, args, environ []string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = environ
	command.Stdout = os.Stdout
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
	configLocation := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-c" || arg == "--config" {
			if i+1 < len(args) {
				i++
				configLocation = args[i]
			}
			continue
		}
		if value, ok := strings.CutPrefix(arg, "--config="); ok {
			configLocation = value
		} else if value, ok := strings.CutPrefix(arg, "-c="); ok {
			configLocation = value
		}
	}
	if configLocation == "" {
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

func stageReportPath(path, name string, pid int) string {
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	return fmt.Sprintf("%s.%d.%s%s", stem, pid, name, ext)
}

func withoutProjectSelection(args []string) ([]string, error) {
	if len(args) != 0 && args[0] == "--" {
		// `just` uses its own `--` to pass Playwright flags through a variadic
		// recipe. Remove that boundary marker before invoking Playwright.
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

func hasJSONReporter(args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		value := ""
		if arg == "--reporter" || arg == "-r" {
			if i+1 < len(args) {
				i++
				value = args[i]
			}
		} else if strings.HasPrefix(arg, "--reporter=") {
			value = strings.TrimPrefix(arg, "--reporter=")
		} else if strings.HasPrefix(arg, "-r=") {
			value = strings.TrimPrefix(arg, "-r=")
		}
		for _, reporter := range strings.Split(value, ",") {
			if strings.TrimSpace(reporter) == "json" {
				return true
			}
		}
	}
	return false
}

type jsonReport struct {
	root   map[string]json.RawMessage
	suites []json.RawMessage
	errors []json.RawMessage
	stats  map[string]json.RawMessage
}

func readReport(path string) (jsonReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return jsonReport{}, err
	}
	var report jsonReport
	if err := json.Unmarshal(data, &report.root); err != nil {
		return jsonReport{}, err
	}
	if err := json.Unmarshal(report.root["suites"], &report.suites); err != nil {
		return jsonReport{}, fmt.Errorf("read suites: %w", err)
	}
	if value := report.root["errors"]; len(value) != 0 {
		if err := json.Unmarshal(value, &report.errors); err != nil {
			return jsonReport{}, fmt.Errorf("read errors: %w", err)
		}
	}
	if value := report.root["stats"]; len(value) != 0 {
		if err := json.Unmarshal(value, &report.stats); err != nil {
			return jsonReport{}, fmt.Errorf("read stats: %w", err)
		}
	}
	return report, nil
}

func combineReports(output string, paths []string) error {
	if len(paths) != len(stages) {
		return fmt.Errorf("got %d stage reports, want %d", len(paths), len(stages))
	}
	combined, err := readReport(paths[0])
	if err != nil {
		return fmt.Errorf("read regular report: %w", err)
	}
	for i, path := range paths[1:] {
		report, err := readReport(path)
		if err != nil {
			return fmt.Errorf("read %s report: %w", stages[i+1].name, err)
		}
		combined.suites = append(combined.suites, report.suites...)
		combined.errors = append(combined.errors, report.errors...)
		combined.stats = addStats(combined.stats, report.stats)
	}
	if err := putJSON(combined.root, "suites", combined.suites); err != nil {
		return err
	}
	if err := putJSON(combined.root, "errors", combined.errors); err != nil {
		return err
	}
	if len(combined.stats) != 0 {
		if err := putJSON(combined.root, "stats", combined.stats); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(combined.root, "", "\t")
	if err != nil {
		return err
	}
	return os.WriteFile(output, append(data, '\n'), 0o644)
}

func addStats(left, right map[string]json.RawMessage) map[string]json.RawMessage {
	if left == nil {
		left = make(map[string]json.RawMessage)
	}
	for key, value := range right {
		if key == "startTime" {
			continue
		}
		var add float64
		if json.Unmarshal(value, &add) != nil {
			if _, exists := left[key]; !exists {
				left[key] = value
			}
			continue
		}
		var current float64
		if existing := left[key]; len(existing) != 0 {
			if json.Unmarshal(existing, &current) != nil {
				continue
			}
		}
		left[key], _ = json.Marshal(current + add)
	}
	return left
}

func putJSON(target map[string]json.RawMessage, key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	target[key] = encoded
	return nil
}

func copyReport(destination io.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(destination, file)
	return err
}
