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
	"regexp"
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
	playwrightHookEnv = "PW_TEST_REPORTER"
)

var blobOutputEnvs = []string{blobOutputEnv, blobOutputDirEnv, blobOutputFileEnv}

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
	parsed, err := parseWrapperArgs(args)
	if err != nil {
		return err
	}
	if envValue(environ, playwrightHookEnv) != "" {
		return fmt.Errorf("%s is reserved by the ordered Playwright runner and must be unset", playwrightHookEnv)
	}

	workingDir, err := filepath.Abs(".")
	if err != nil {
		return fmt.Errorf("resolve Playwright working directory: %w", err)
	}
	baseRun := envValue(environ, "SKGO_E2E_RUN")
	if baseRun == "" {
		baseRun = "run"
	}
	selectedReporter, hookReporter, err := finalReporter(parsed, workingDir)
	if err != nil {
		return err
	}

	if callerSelectedJSON(parsed) {
		jsonPath, known, err := knownJSONDestination(parsed, environ, workingDir)
		if err != nil {
			return fmt.Errorf("resolve Playwright JSON output destination: %w", err)
		}
		if known {
			if err := removePriorReportFile(jsonPath); err != nil {
				return fmt.Errorf("remove prior Playwright JSON report: %w", err)
			}
		}
	}

	scratchDir, err := os.MkdirTemp("", "skgo-e2e-stage-scratch-")
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
		stageEnv = unsetEnv(stageEnv, playwrightHookEnv)
		for _, key := range blobOutputEnvs {
			stageEnv = unsetEnv(stageEnv, key)
		}
		stageEnv = setEnv(stageEnv, blobOutputDirEnv, blobDir)
		stageEnv = setEnv(stageEnv, blobOutputFileEnv, blobPath)

		commandArgs := []string{"test"}
		for _, project := range current.projects {
			commandArgs = append(commandArgs, "--project="+project)
		}
		stageArgs := testArgsWithReporter(parsed.stageArgs, "blob")
		commandArgs = append(commandArgs, stageArgs...)
		if err := invoke(ctx, "./node_modules/.bin/playwright", commandArgs, stageEnv); err != nil {
			if ctx.Err() != nil {
				return errors.Join(append(failures, ctx.Err())...)
			}
			failures = append(failures, fmt.Errorf("%s Playwright stage failed: %w", current.name, err))
		}
	}

	configArgs, err := mergeConfigArgs(parsed.config, parsed.configSpecified, workingDir)
	if err != nil {
		return errors.Join(append(failures, err)...)
	}
	mergeInputDir, err := retainBlobReports(blobPaths)
	if err != nil {
		failures = append(failures, fmt.Errorf("retain Playwright blob reports: %w", err))
		return errors.Join(failures...)
	}
	mergeArgs := []string{"merge-reports", mergeInputDir}
	mergeArgs = append(mergeArgs, configArgs...)
	mergeEnv := unsetEnv(environ, playwrightHookEnv)
	if hookReporter != "" {
		mergeEnv = setEnv(mergeEnv, playwrightHookEnv, hookReporter)
	}
	if selectedReporter != "" {
		mergeArgs = append(mergeArgs, "--reporter", selectedReporter)
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(append(failures, err)...)
	}
	if err := invoke(ctx, "./node_modules/.bin/playwright", mergeArgs, mergeEnv); err != nil {
		if ctx.Err() != nil {
			failures = append(failures, ctx.Err())
		} else {
			failures = append(failures, fmt.Errorf("merge Playwright reports failed: %w", err))
		}
	}
	if _, err := archiveBlobReports(mergeInputDir, workingDir); err != nil {
		failures = append(failures, fmt.Errorf("archive Playwright blob reports: %w", err))
	}
	return errors.Join(failures...)
}

func execPlaywright(ctx context.Context, name string, args, environ []string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = environ
	command.Stdout = os.Stdout
	if len(args) != 0 && args[0] == "test" {
		// Keep stage progress off stdout so a JSON-only final reporter can emit
		// one parseable native document there.
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

type optionArity uint8

const (
	optionFlag optionArity = iota
	optionRequired
	optionOptional
	optionVariadic
)

type wrapperOption struct {
	arity optionArity
	owned string
}

type parsedInvocation struct {
	stageArgs       []string
	reporter        string
	addedReporter   string
	config          string
	configSpecified bool
}

// These are Playwright 1.63.0's test options and operand roles. Tracking every
// declared required/optional/variadic operand prevents a flag-looking value
// (for example --grep --reporter=json) from being reinterpreted by the wrapper.
var nativeTestOptions = map[string]wrapperOption{
	"--add-reporter":         {arity: optionRequired, owned: "addition"},
	"--browser":              {arity: optionRequired},
	"-c":                     {arity: optionRequired, owned: "config"},
	"--config":               {arity: optionRequired, owned: "config"},
	"--debug":                {arity: optionOptional},
	"--fail-on-flaky-tests":  {arity: optionFlag},
	"--forbid-only":          {arity: optionFlag},
	"--fully-parallel":       {arity: optionFlag},
	"--global-timeout":       {arity: optionRequired},
	"-g":                     {arity: optionRequired},
	"--grep":                 {arity: optionRequired},
	"-G":                     {arity: optionRequired},
	"--grep-invert":          {arity: optionRequired},
	"--headed":               {arity: optionFlag},
	"--ignore-snapshots":     {arity: optionFlag},
	"--last-failed":          {arity: optionFlag},
	"--last-failed-file":     {arity: optionRequired},
	"--list":                 {arity: optionFlag},
	"--max-failures":         {arity: optionRequired},
	"--no-deps":              {arity: optionFlag},
	"--output":               {arity: optionRequired},
	"--only-changed":         {arity: optionOptional},
	"--pass-with-no-tests":   {arity: optionFlag},
	"--project":              {arity: optionVariadic, owned: "project"},
	"--quiet":                {arity: optionFlag},
	"--repeat-each":          {arity: optionRequired},
	"--reporter":             {arity: optionRequired, owned: "reporter"},
	"--retries":              {arity: optionRequired},
	"--run-agents":           {arity: optionRequired},
	"--shard":                {arity: optionRequired},
	"--test-list":            {arity: optionRequired},
	"--test-list-invert":     {arity: optionRequired},
	"--timeout":              {arity: optionRequired},
	"--trace":                {arity: optionRequired},
	"--tsconfig":             {arity: optionRequired},
	"--ui":                   {arity: optionFlag},
	"--ui-host":              {arity: optionRequired},
	"--ui-port":              {arity: optionRequired},
	"-u":                     {arity: optionOptional},
	"--update-snapshots":     {arity: optionOptional},
	"--update-source-method": {arity: optionRequired},
	"-j":                     {arity: optionRequired},
	"--workers":              {arity: optionRequired},
	"-x":                     {arity: optionFlag},
	"--help":                 {arity: optionFlag},
	"-h":                     {arity: optionFlag},
	"--version":              {arity: optionFlag},
	"-V":                     {arity: optionFlag},
}

func parseWrapperArgs(args []string) (parsedInvocation, error) {
	parsed := parsedInvocation{stageArgs: make([]string, 0, len(args))}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			parsed.stageArgs = append(parsed.stageArgs, args[i:]...)
			break
		}
		if isShortCluster(arg) {
			next, handled, err := parseShortCluster(args, i, &parsed)
			if err != nil {
				return parsedInvocation{}, err
			}
			if handled {
				i = next
				continue
			}
			parsed.stageArgs = append(parsed.stageArgs, arg)
			continue
		}

		name, value, inline, ok := nativeOptionToken(arg)
		if !ok {
			parsed.stageArgs = append(parsed.stageArgs, arg)
			continue
		}
		option := nativeTestOptions[name]
		if option.arity == optionFlag {
			parsed.stageArgs = append(parsed.stageArgs, arg)
			continue
		}
		if option.arity == optionOptional {
			if !inline && i+1 < len(args) && canConsumeOptional(args[i+1]) {
				parsed.stageArgs = append(parsed.stageArgs, arg, args[i+1])
				i++
			} else {
				parsed.stageArgs = append(parsed.stageArgs, arg)
			}
			continue
		}

		if option.arity == optionVariadic {
			if !inline {
				if i+1 == len(args) {
					return parsedInvocation{}, fmt.Errorf("%s needs a project name", name)
				}
				value = args[i+1]
				if value == "--" {
					return parsedInvocation{}, errors.New("a literal -- project operand is unsupported by the ordered Playwright runner")
				}
				if err := validateCallerProject(value); err != nil {
					return parsedInvocation{}, err
				}
				i++
				for i+1 < len(args) && args[i+1] != "--" && !isCommanderOption(args[i+1]) {
					if err := validateCallerProject(args[i+1]); err != nil {
						return parsedInvocation{}, err
					}
					i++
				}
			} else if err := validateCallerProject(value); err != nil {
				return parsedInvocation{}, err
			}
			continue
		}

		if !inline {
			if i+1 == len(args) {
				return parsedInvocation{}, fmt.Errorf("%s needs a value", name)
			}
			value = args[i+1]
			if value == "--" {
				return parsedInvocation{}, fmt.Errorf("a literal -- operand to %s is unsupported by the ordered Playwright runner", name)
			}
			i++
		}
		switch option.owned {
		case "reporter":
			parsed.reporter = value
		case "addition":
			parsed.addedReporter = value
		case "config":
			parsed.config, parsed.configSpecified = value, true
			if inline {
				parsed.stageArgs = append(parsed.stageArgs, arg)
			} else {
				parsed.stageArgs = append(parsed.stageArgs, arg, value)
			}
		default:
			if inline {
				parsed.stageArgs = append(parsed.stageArgs, arg)
			} else {
				parsed.stageArgs = append(parsed.stageArgs, arg, value)
			}
		}
	}
	return parsed, nil
}

func isShortCluster(arg string) bool {
	return len(arg) > 2 && arg[0] == '-' && arg[1] != '-'
}

// parseShortCluster mirrors Commander short groups: flags are consumed from
// left to right, and the first required/optional value option owns the rest
// of the token or its next argv operand.
func parseShortCluster(args []string, index int, parsed *parsedInvocation) (int, bool, error) {
	arg := args[index]
	for pos := 1; pos < len(arg); pos++ {
		name := "-" + arg[pos:pos+1]
		option, ok := nativeTestOptions[name]
		if !ok {
			return index, false, nil
		}
		if option.arity == optionFlag {
			continue
		}

		value := ""
		inline := pos+1 < len(arg)
		if inline {
			value = arg[pos+1:]
		} else if option.arity == optionRequired {
			if index+1 == len(args) {
				return index, true, fmt.Errorf("%s needs a value", name)
			}
			value = args[index+1]
			if value == "--" {
				return index, true, fmt.Errorf("a literal -- operand to %s is unsupported by the ordered Playwright runner", name)
			}
		} else if option.arity == optionOptional && index+1 < len(args) && canConsumeOptional(args[index+1]) {
			value = args[index+1]
			inline = false
		} else {
			parsed.stageArgs = append(parsed.stageArgs, arg)
			return index, true, nil
		}

		if option.owned == "config" {
			parsed.config, parsed.configSpecified = value, true
		}
		parsed.stageArgs = append(parsed.stageArgs, arg)
		if !inline {
			parsed.stageArgs = append(parsed.stageArgs, value)
			return index + 1, true, nil
		}
		return index, true, nil
	}
	parsed.stageArgs = append(parsed.stageArgs, arg)
	return index, true, nil
}

func nativeOptionToken(arg string) (name, value string, inline, ok bool) {
	if strings.HasPrefix(arg, "--") {
		if candidate, candidateValue, hasValue := strings.Cut(arg, "="); hasValue {
			option, known := nativeTestOptions[candidate]
			if known && (option.arity == optionRequired || option.arity == optionOptional || option.arity == optionVariadic) {
				return candidate, candidateValue, true, true
			}
			return "", "", false, false
		}
		_, ok = nativeTestOptions[arg]
		return arg, "", false, ok
	}
	if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
		if _, known := nativeTestOptions[arg]; known {
			return arg, "", false, true
		}
	}
	return "", "", false, false
}

func isCommanderOption(arg string) bool {
	if arg == "--" {
		return true
	}
	return len(arg) > 1 && arg[0] == '-' && !negativeNumberArg(arg)
}

var commanderNegativeNumber = regexp.MustCompile(`^-(\d+|\d*\.\d+)(e[+-]?\d+)?$`)

func negativeNumberArg(arg string) bool {
	return commanderNegativeNumber.MatchString(arg)
}

func canConsumeOptional(arg string) bool {
	return !isCommanderOption(arg)
}

func validateCallerProject(project string) error {
	if project == "" {
		return errors.New("--project needs a non-empty project name in the ordered Playwright runner")
	}
	if strings.HasPrefix(project, "-") {
		return fmt.Errorf("flag-looking --project operand %q is unsupported by the ordered Playwright runner", project)
	}
	return nil
}

func mergeConfigArgs(config string, configured bool, workingDir string) ([]string, error) {
	if !configured || config == "" {
		// merge-reports otherwise loads an empty config and loses the config
		// Playwright would discover from the test command's working directory.
		config = workingDir
	} else {
		var err error
		config, err = resolveFrom(workingDir, config)
		if err != nil {
			return nil, fmt.Errorf("resolve Playwright config location: %w", err)
		}
	}
	return []string{"--config", config}, nil
}

func finalReporter(parsed parsedInvocation, workingDir string) (string, string, error) {
	if parsed.reporter != "" {
		if parsed.addedReporter == "" {
			return parsed.reporter, "", nil
		}
		return parsed.reporter + "," + parsed.addedReporter, "", nil
	}
	if parsed.addedReporter == "" {
		return "", "", nil
	}
	ids := strings.Split(parsed.addedReporter, ",")
	if len(ids) != 1 {
		return "", "", fmt.Errorf("adding multiple reporters to configured/default reporters is unsupported; supply a non-empty --reporter replacement to use native CSV selection")
	}
	hook, err := hookReporterID(parsed.addedReporter, workingDir)
	return "", hook, err
}

func hookReporterID(id, workingDir string) (string, error) {
	if _, ok := nativeBuiltinReporters[id]; ok {
		return id, nil
	}
	path, err := resolveFrom(workingDir, id)
	if err != nil {
		return "", fmt.Errorf("resolve added Playwright reporter path: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() && !info.IsDir() {
		return "", fmt.Errorf("adding package reporter %q to configured/default reporters is unsupported; use a non-empty --reporter replacement or an existing cwd-local/absolute reporter path", id)
	}
	return path, nil
}

var nativeBuiltinReporters = map[string]struct{}{
	"list": {}, "line": {}, "dot": {}, "json": {}, "junit": {},
	"null": {}, "github": {}, "html": {}, "blob": {}, "perfetto": {},
}

func testArgsWithReporter(args []string, reporter string) []string {
	result := append([]string(nil), args...)
	owned := "--reporter=" + reporter
	for i, arg := range result {
		if arg == "--" {
			result = append(result[:i], append([]string{owned}, result[i:]...)...)
			return result
		}
	}
	return append(result, owned)
}

func callerSelectedJSON(parsed parsedInvocation) bool {
	for _, selection := range []string{parsed.reporter, parsed.addedReporter} {
		for _, reporter := range strings.Split(selection, ",") {
			if reporter == "json" {
				return true
			}
		}
	}
	return false
}

// knownJSONDestination mirrors only paths whose authority is explicit in the
// caller's JSON selection. Configured reporter outputFile tuples remain opaque
// and are deliberately not guessed or deleted.
func knownJSONDestination(parsed parsedInvocation, environ []string, workingDir string) (string, bool, error) {
	if !callerSelectedJSON(parsed) {
		return "", false, nil
	}
	if outputFile := envValue(environ, jsonOutputFileEnv); outputFile != "" {
		path, err := resolveFrom(workingDir, outputFile)
		return path, true, err
	}
	outputName := envValue(environ, jsonOutputEnv)
	if outputName == "" {
		return "", false, nil
	}
	outputDir := envValue(environ, jsonOutputDirEnv)
	if outputDir == "" {
		var err error
		outputDir, err = selectedConfigDir(parsed, workingDir)
		if err != nil {
			return "", false, err
		}
	} else {
		var err error
		outputDir, err = resolveFrom(workingDir, outputDir)
		if err != nil {
			return "", false, err
		}
	}
	path, err := resolveFrom(outputDir, outputName)
	return path, true, err
}

// selectedConfigDir follows Playwright 1.63's config-file/directory lookup
// without loading or interpreting the JavaScript/TypeScript config itself.
func selectedConfigDir(parsed parsedInvocation, workingDir string) (string, error) {
	configPath := workingDir
	if parsed.configSpecified && parsed.config != "" {
		var err error
		configPath, err = resolveFrom(workingDir, parsed.config)
		if err != nil {
			return "", err
		}
	}
	info, err := os.Stat(configPath)
	if err != nil {
		return "", fmt.Errorf("inspect selected Playwright config location %q: %w", configPath, err)
	}
	if !info.IsDir() {
		return filepath.Dir(configPath), nil
	}
	for _, extension := range []string{".ts", ".js", ".mts", ".mjs", ".cts", ".cjs"} {
		candidate := filepath.Join(configPath, "playwright.config"+extension)
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Dir(candidate), nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return configPath, nil
}

func removePriorReportFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("known report destination %q is a directory", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("known report destination %q is not a regular file", path)
	}
	return os.Remove(path)
}

// resolveFrom matches Node's path.resolve(base, value): an absolute value
// overrides base, while a relative value is joined to it.
func resolveFrom(base, value string) (string, error) {
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Abs(filepath.Join(base, value))
}

func retainBlobReports(paths []string) (string, error) {
	if len(paths) != len(stages) {
		return "", fmt.Errorf("got %d Playwright blob reports, want %d", len(paths), len(stages))
	}
	retainedDir, err := os.MkdirTemp("", "skgo-e2e-merge-input-")
	if err != nil {
		return "", fmt.Errorf("create native merge-input directory: %w", err)
	}
	for index, source := range paths {
		destination := filepath.Join(retainedDir, stages[index].name+".zip")
		if err := copyFile(destination, source); err != nil {
			_ = os.RemoveAll(retainedDir)
			return "", fmt.Errorf("copy %s blob report: %w", stages[index].name, err)
		}
	}
	return retainedDir, nil
}

// archiveBlobReports copies the native merge input only after final reporters
// have returned. HTML and blob reporters may clear their configured outputDir,
// so the source must live outside cwd/test-results and remain at the paths
// native JSON/HTML reporters emit. The archive is an additional byte-identical
// copy for the repository's failure-artifact collection.
func archiveBlobReports(sourceDir, workingDir string) (string, error) {
	sourceInfo, err := os.Lstat(sourceDir)
	if err != nil {
		return "", fmt.Errorf("inspect native merge input: %w", err)
	}
	if !sourceInfo.IsDir() {
		return "", fmt.Errorf("native merge input %q is not a directory", sourceDir)
	}
	artifactRoot := filepath.Join(workingDir, "test-results")
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		return "", fmt.Errorf("create test-results artifact directory: %w", err)
	}
	archiveDir, err := os.MkdirTemp(artifactRoot, "skgo-e2e-blobs-")
	if err != nil {
		return "", fmt.Errorf("create retained artifact directory: %w", err)
	}
	if err := copyDirectoryTree(sourceDir, archiveDir); err != nil {
		_ = os.RemoveAll(archiveDir)
		return "", fmt.Errorf("copy native merge input: %w", err)
	}
	return archiveDir, nil
}

func copyDirectoryTree(sourceDir, destinationDir string) error {
	return filepath.WalkDir(sourceDir, func(source string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, source)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		destination := filepath.Join(destinationDir, relative)
		info, err := os.Lstat(source)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.MkdirAll(destination, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported native merge artifact %q with mode %s", source, info.Mode())
		}
		return copyFileMode(destination, source, info.Mode().Perm())
	})
}

func copyFile(destination, source string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	return copyFileMode(destination, source, info.Mode().Perm())
}

func copyFileMode(destination, source string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
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
