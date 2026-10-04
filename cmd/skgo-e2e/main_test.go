package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFailedRegularStageStillMergesBothBlobsOnce(t *testing.T) {
	workingDir := chdirTemp(t)
	config := filepath.Join(t.TempDir(), "external", "playwright.config.js")
	callerBlob := filepath.Join(t.TempDir(), "caller.zip")
	jsonFile := filepath.Join(t.TempDir(), "current.json")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsonFile, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	regularFailure := errors.New("regular assertion failed")
	args := []string{
		"-c", config,
		"--grep", "--reporter=json", // required value must not be parsed as a reporter flag
		"--grep-invert=route|module",
		"--reporter=dot", "--reporter", "json",
		"--add-reporter=list", "--add-reporter=json",
		"--project=source-edit", "--project", "chromium", "noscript",
	}
	environ := []string{
		jsonOutputFileEnv + "=" + jsonFile,
		jsonOutputDirEnv + "=ignored",
		jsonOutputEnv + "=ignored.json",
		blobOutputFileEnv + "=" + callerBlob,
		blobOutputDirEnv + "=" + filepath.Dir(callerBlob),
		blobOutputEnv + "=caller-name.zip",
		"SKGO_E2E_RUN=proof",
	}
	var stageNames []string
	mergeCalls := 0
	var retainedDir string
	stageBlobDirs := map[string]string{}
	err := runWithCleanup(t, context.Background(), args, environ, func(_ context.Context, name string, commandArgs, commandEnv []string) error {
		if name != "./node_modules/.bin/playwright" {
			t.Fatalf("playwright path = %q", name)
		}
		switch commandArgs[0] {
		case "test":
			stage := envValue(commandEnv, "SKGO_E2E_STAGE")
			stageNames = append(stageNames, stage)
			if got := envValue(commandEnv, "SKGO_E2E_RUN"); got != "proof-"+stage {
				t.Fatalf("stage run label = %q for %s", got, stage)
			}
			if !contains(commandArgs, "--reporter=blob") || count(commandArgs, "--reporter=blob") != 1 {
				t.Fatalf("stage reporter = %v, want only owned blob", commandArgs)
			}
			for _, arg := range commandArgs {
				if arg == "--reporter" || arg == "--add-reporter" || strings.HasPrefix(arg, "--add-reporter=") {
					t.Fatalf("caller reporter selection leaked into stage: %v", commandArgs)
				}
			}
			if !contains(commandArgs, "--grep") || !contains(commandArgs, "--reporter=json") || !contains(commandArgs, "--grep-invert=route|module") {
				t.Fatalf("native filter operands were changed: %v", commandArgs)
			}
			if contains(commandArgs, "--project=source-edit") && stage != "source-edit" {
				t.Fatalf("caller project escaped wrapper ownership: %v", commandArgs)
			}
			for _, key := range blobOutputEnvs {
				if key == blobOutputEnv && envValue(commandEnv, key) != "" {
					t.Fatalf("caller blob name leaked into stage: %q", envValue(commandEnv, key))
				}
			}
			if envValue(commandEnv, playwrightHookEnv) != "" {
				t.Fatalf("internal reporter hook leaked into stage: %v", commandEnv)
			}
			blob := envValue(commandEnv, blobOutputFileEnv)
			if !filepath.IsAbs(blob) || blob == callerBlob || filepath.Dir(blob) != envValue(commandEnv, blobOutputDirEnv) {
				t.Fatalf("stage blob output is not isolated: %v", commandEnv)
			}
			stageBlobDirs[stage] = filepath.Dir(blob)
			if err := os.WriteFile(blob, []byte("blob-"+stage), 0o600); err != nil {
				t.Fatal(err)
			}
			if stage == "regular" {
				return regularFailure
			}
			return nil
		case "merge-reports":
			mergeCalls++
			if len(commandArgs) < 2 {
				t.Fatalf("merge has no retained blob directory: %v", commandArgs)
			}
			retainedDir = commandArgs[1]
			if !filepath.IsAbs(retainedDir) || !strings.Contains(retainedDir, "skgo-e2e-merge-input-") || pathIsWithin(retainedDir, filepath.Join(workingDir, "test-results")) {
				t.Fatalf("native merge-input path = %q; it must be unique OS temp, outside cwd/test-results", retainedDir)
			}
			for _, stage := range []string{"regular", "source-edit"} {
				if pathIsWithin(retainedDir, stageBlobDirs[stage]) || pathIsWithin(stageBlobDirs[stage], retainedDir) {
					t.Fatalf("stage scratch and native merge input overlap: stage=%q stageDir=%q mergeDir=%q", stage, stageBlobDirs[stage], retainedDir)
				}
			}
			if !contains(commandArgs, "--config") || !contains(commandArgs, config) || !contains(commandArgs, "--reporter") || !contains(commandArgs, "json,json") {
				t.Fatalf("final reporter/config selection = %v", commandArgs)
			}
			if envValue(commandEnv, playwrightHookEnv) != "" {
				t.Fatalf("replacement reporter should not use internal hook: %v", commandEnv)
			}
			if envValue(commandEnv, jsonOutputFileEnv) != jsonFile || envValue(commandEnv, blobOutputFileEnv) != callerBlob {
				t.Fatalf("final merge did not retain caller environment: %v", commandEnv)
			}
			if _, err := os.Stat(jsonFile); !os.IsNotExist(err) {
				t.Fatalf("stale known JSON FILE remains before native merge, stat error = %v", err)
			}
			for _, stage := range []string{"regular", "source-edit"} {
				data, err := os.ReadFile(filepath.Join(retainedDir, stage+".zip"))
				if err != nil || string(data) != "blob-"+stage {
					t.Fatalf("retained %s blob = %q, err=%v", stage, data, err)
				}
			}
			resource := filepath.Join(retainedDir, "resources", "attached.txt")
			if err := os.MkdirAll(filepath.Dir(resource), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(resource, []byte("native attachment bytes"), 0o640); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(retainedDir, "report.jsonl"), []byte("native report events"), 0o640); err != nil {
				t.Fatal(err)
			}
			// A final HTML/blob reporter is allowed to clear cwd/test-results.
			if err := os.RemoveAll("test-results"); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll("test-results", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join("test-results", "index.html"), []byte("final reporter output"), 0o640); err != nil {
				t.Fatal(err)
			}
			return os.WriteFile(jsonFile, []byte("merged"), 0o600)
		default:
			t.Fatalf("unexpected Playwright subcommand: %v", commandArgs)
			return nil
		}
	})
	if err == nil || !errors.Is(err, regularFailure) || !strings.Contains(err.Error(), "regular Playwright stage failed") {
		t.Fatalf("run error = %v, want retained regular failure", err)
	}
	if !reflect.DeepEqual(stageNames, []string{"regular", "source-edit"}) || mergeCalls != 1 {
		t.Fatalf("stages=%v merges=%d; want ordered stages and one merge", stageNames, mergeCalls)
	}
	for _, stage := range []string{"regular", "source-edit"} {
		if _, err := os.Stat(stageBlobDirs[stage]); !os.IsNotExist(err) {
			t.Fatalf("stage scratch %q survived return, stat error=%v", stageBlobDirs[stage], err)
		}
	}
	if retainedDir == "" {
		t.Fatal("native merge-input directory was not captured")
	}
	for relative, want := range map[string]string{
		"regular.zip":     "blob-regular",
		"source-edit.zip": "blob-source-edit",
		"report.jsonl":    "native report events",
		filepath.Join("resources", "attached.txt"): "native attachment bytes",
	} {
		original, err := os.ReadFile(filepath.Join(retainedDir, relative))
		if err != nil || string(original) != want {
			t.Fatalf("original merge input %s = %q err=%v want %q", relative, original, err, want)
		}
	}
	archives, err := filepath.Glob(filepath.Join(workingDir, "test-results", "skgo-e2e-blobs-*"))
	if err != nil || len(archives) != 1 {
		t.Fatalf("retained artifact directories=%v err=%v, want one post-reporter copy", archives, err)
	}
	for relative, want := range map[string]string{
		"regular.zip":     "blob-regular",
		"source-edit.zip": "blob-source-edit",
		"report.jsonl":    "native report events",
		filepath.Join("resources", "attached.txt"): "native attachment bytes",
	} {
		archived, err := os.ReadFile(filepath.Join(archives[0], relative))
		if err != nil || string(archived) != want {
			t.Fatalf("archived artifact %s = %q err=%v want %q", relative, archived, err, want)
		}
	}
	if data, err := os.ReadFile(filepath.Join(workingDir, "test-results", "index.html")); err != nil || string(data) != "final reporter output" {
		t.Fatalf("final reporter output was lost during post-merge archive: %q err=%v", data, err)
	}
	if data, err := os.ReadFile(jsonFile); err != nil || string(data) != "merged" {
		t.Fatalf("final file = %q, err=%v", data, err)
	}
	if workingDir == "" {
		t.Fatal("working directory not initialized")
	}
}

func TestConfiguredBaseUsesNativeHookForOneAddedReporter(t *testing.T) {
	chdirTemp(t)
	configDir := filepath.Join(t.TempDir(), "external-config")
	config := filepath.Join(configDir, "playwright.config.js")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("export default {};\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mergeCalls := 0
	err := runWithCleanup(t, context.Background(), []string{"-c" + config, "--add-reporter=json"}, []string{jsonOutputEnv + "=env-name.json"}, writeBlobRunner(t, func(args, environ []string) {
		if args[0] != "merge-reports" {
			return
		}
		mergeCalls++
		if contains(args, "--reporter") {
			t.Fatalf("configured reporter base was replaced: %v", args)
		}
		if got := envValue(environ, playwrightHookEnv); got != "json" {
			t.Fatalf("PW_TEST_REPORTER = %q, want json", got)
		}
		if envValue(environ, jsonOutputEnv) != "env-name.json" {
			t.Fatalf("native JSON destination env was changed: %v", environ)
		}
		if !contains(args, config) {
			t.Fatalf("selected external config not passed to merge: %v", args)
		}
	}))
	if err != nil || mergeCalls != 1 {
		t.Fatalf("run error=%v, merge calls=%d", err, mergeCalls)
	}
}

func TestJSONEnvironmentAloneDoesNotSelectOrReplaceReporter(t *testing.T) {
	for _, env := range [][]string{
		{jsonOutputFileEnv + "=report.json"},
		{jsonOutputDirEnv + "=reports", jsonOutputEnv + "=report.json"},
		{jsonOutputEnv + "=report.json"},
		{jsonOutputDirEnv + "=reports"},
		{jsonOutputFileEnv + "=", jsonOutputDirEnv + "=", jsonOutputEnv + "="},
	} {
		t.Run(strings.Join(env, ";"), func(t *testing.T) {
			chdirTemp(t)
			mergeCalls := 0
			err := runWithCleanup(t, context.Background(), nil, env, writeBlobRunner(t, func(args, environ []string) {
				if args[0] != "merge-reports" {
					return
				}
				mergeCalls++
				if contains(args, "--reporter") || envValue(environ, playwrightHookEnv) != "" {
					t.Fatalf("JSON env selected a replacement reporter: args=%v env=%v", args, environ)
				}
				for _, entry := range env {
					key, value, ok := strings.Cut(entry, "=")
					if ok && envValue(environ, key) != value {
						t.Fatalf("merge env %s = %q, want %q", key, envValue(environ, key), value)
					}
				}
			}))
			if err != nil || mergeCalls != 1 {
				t.Fatalf("run error=%v, merge calls=%d", err, mergeCalls)
			}
		})
	}
}

func TestConfiguredBaseAdditionBoundsAndInternalHookOwnership(t *testing.T) {
	chdirTemp(t)
	for _, test := range []struct {
		name string
		env  []string
		args []string
		want string
	}{
		{name: "caller hook rejected", env: []string{playwrightHookEnv + "=html"}},
		{name: "multiple configured-base additions rejected", args: []string{"--add-reporter=json,dot"}},
		{name: "bare package addition rejected", args: []string{"--add-reporter=example-reporter"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			err := runWithCleanup(t, context.Background(), test.args, test.env, func(context.Context, string, []string, []string) error {
				calls++
				return nil
			})
			if err == nil || calls != 0 {
				t.Fatalf("error=%v calls=%d, want a pre-stage compatibility error", err, calls)
			}
		})
	}

	localReporter := filepath.Join("reporter", "custom.cjs")
	if err := os.MkdirAll(filepath.Dir(localReporter), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localReporter, []byte("module.exports = class {};\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	parsed, err := parseWrapperArgs([]string{"--add-reporter", localReporter})
	if err != nil {
		t.Fatal(err)
	}
	_, hook, err := finalReporter(parsed, mustGetwd(t))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(localReporter)
	if err != nil {
		t.Fatal(err)
	}
	if hook != want {
		t.Fatalf("local hook path = %q, want absolute %q", hook, want)
	}
	combined, hook, err := finalReporter(parsedInvocation{reporter: "json,json", addedReporter: "dot,json"}, mustGetwd(t))
	if err != nil || combined != "json,json,dot,json" || hook != "" {
		t.Fatalf("combined reporter=%q hook=%q error=%v", combined, hook, err)
	}
}

func TestNativeTokenRolesAndBoundaries(t *testing.T) {
	args := []string{
		"--grep", "--reporter=json",
		"--grep-invert=left|right",
		"--reporter=json", "--reporter=",
		"--add-reporter=html", "--add-reporter", "",
		"-c", "first.config.js", "--config=",
		"--project", "chromium", "noscript", "--project=source-edit", "literal-file.regex",
		"--", "--reporter=json", "--config=after-boundary", "--project", "source-edit",
	}
	parsed, err := parseWrapperArgs(args)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.reporter != "" || parsed.addedReporter != "" {
		t.Fatalf("last empty reporter values did not clear prior values: %+v", parsed)
	}
	if !parsed.configSpecified || parsed.config != "" {
		t.Fatalf("last empty config did not restore cwd discovery: %+v", parsed)
	}
	want := []string{
		"--grep", "--reporter=json",
		"--grep-invert=left|right",
		"-c", "first.config.js", "--config=",
		"literal-file.regex",
		"--", "--reporter=json", "--config=after-boundary", "--project", "source-edit",
	}
	if !reflect.DeepEqual(parsed.stageArgs, want) {
		t.Fatalf("stage args = %#v, want %#v", parsed.stageArgs, want)
	}
	got := testArgsWithReporter(parsed.stageArgs, "blob")
	gotDash, wantDash := indexOf(got, "--"), indexOf(want, "--")
	if gotDash < 0 || wantDash < 0 || !reflect.DeepEqual(got[gotDash+1:], want[wantDash+1:]) {
		t.Fatalf("native separator suffix changed after owned reporter insertion: %v", got)
	}
}

func TestSeparatedAndEqualsProjectOperandsFollowNativeArity(t *testing.T) {
	parsed, err := parseWrapperArgs([]string{
		"--project", "chromium", "noscript", "--grep", "a|b",
		"--project=source-edit", "fixture.*",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--grep", "a|b", "fixture.*"}
	if !reflect.DeepEqual(parsed.stageArgs, want) {
		t.Fatalf("stage args = %#v, want %#v", parsed.stageArgs, want)
	}
	for _, args := range [][]string{
		{"--project"},
		{"--project="},
		{"--project", ""},
		{"--project", "--reporter=json"},
		{"--project", "chromium", "--project"},
	} {
		if _, err := parseWrapperArgs(args); err == nil {
			t.Errorf("parseWrapperArgs(%q) unexpectedly succeeded", args)
		}
	}
	for _, alias := range [][]string{{"-p"}, {"-p=chromium"}, {"-r=json"}} {
		parsed, err := parseWrapperArgs(alias)
		if err != nil || !reflect.DeepEqual(parsed.stageArgs, alias) || parsed.reporter != "" {
			t.Errorf("unsupported native alias %q was accepted or rewritten: parsed=%+v err=%v", alias, parsed, err)
		}
	}
}

func TestRequiredOperandsCannotBeFilledByOwnedArguments(t *testing.T) {
	for _, args := range [][]string{
		{"--reporter"}, {"--add-reporter"}, {"--config"}, {"-c"}, {"--grep"}, {"--project"},
		{"--reporter", "--"}, {"--add-reporter", "--"}, {"--config", "--"}, {"--grep", "--"},
	} {
		if _, err := parseWrapperArgs(args); err == nil {
			t.Errorf("parseWrapperArgs(%q) unexpectedly succeeded", args)
		}
	}
	parsed, err := parseWrapperArgs([]string{"--grep", "--config=/literal", "--config", "--reporter=json"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.reporter != "" || parsed.config != "--reporter=json" {
		t.Fatalf("required flag-looking operands were scanned as options: %+v", parsed)
	}
	parsed, err = parseWrapperArgs([]string{"-xg", "--reporter=json"})
	if err != nil || parsed.reporter != "" || !reflect.DeepEqual(parsed.stageArgs, []string{"-xg", "--reporter=json"}) {
		t.Fatalf("boolean short cluster lost its required grep operand: parsed=%+v err=%v", parsed, err)
	}
	parsed, err = parseWrapperArgs([]string{"-xc/path/playwright.config.js"})
	if err != nil || parsed.config != "/path/playwright.config.js" || !reflect.DeepEqual(parsed.stageArgs, []string{"-xc/path/playwright.config.js"}) {
		t.Fatalf("short cluster config operand was not retained: parsed=%+v err=%v", parsed, err)
	}
}

func TestMergeConfigAndReporterNativeComposition(t *testing.T) {
	workingDir := filepath.Join(string(filepath.Separator), "task", "fixture")
	for _, test := range []struct {
		args []string
		want string
	}{
		{args: []string{"-c/path/config.js"}, want: filepath.Join(string(filepath.Separator), "path", "config.js")},
		{args: []string{"-c", "/path/config.js"}, want: filepath.Join(string(filepath.Separator), "path", "config.js")},
		{args: []string{"--config", "/path/config.js"}, want: filepath.Join(string(filepath.Separator), "path", "config.js")},
		{args: []string{"--config=/path/config.js"}, want: filepath.Join(string(filepath.Separator), "path", "config.js")},
		{args: []string{"-c=literal"}, want: filepath.Join(workingDir, "=literal")},
	} {
		parsed, err := parseWrapperArgs(test.args)
		if err != nil {
			t.Fatal(err)
		}
		got, err := mergeConfigArgs(parsed.config, parsed.configSpecified, workingDir)
		if err != nil || !reflect.DeepEqual(got, []string{"--config", test.want}) {
			t.Errorf("merge config for %q = %v, err=%v; want %q", test.args, got, err, test.want)
		}
	}
	parsed, err := parseWrapperArgs([]string{"--reporter=json,json", "--add-reporter=dot,json"})
	if err != nil {
		t.Fatal(err)
	}
	got, hook, err := finalReporter(parsed, workingDir)
	if err != nil || got != "json,json,dot,json" || hook != "" {
		t.Fatalf("CSV composition = %q, hook=%q, err=%v", got, hook, err)
	}
	parsed, err = parseWrapperArgs([]string{"--add-reporter=json,dot"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := finalReporter(parsed, workingDir); err == nil || !strings.Contains(err.Error(), "multiple reporters") {
		t.Fatalf("multi-addition on config base error = %v", err)
	}
}

func TestStageAndMergeErrorsRemainAggregate(t *testing.T) {
	chdirTemp(t)
	regularFailure := errors.New("regular")
	sourceFailure := errors.New("source-edit")
	mergeFailure := errors.New("native merge")
	for _, test := range []struct {
		name       string
		regularErr error
		sourceErr  error
		mergeErr   error
		wantError  bool
	}{
		{name: "all succeed"},
		{name: "regular fails", regularErr: regularFailure, wantError: true},
		{name: "source-edit fails", sourceErr: sourceFailure, wantError: true},
		{name: "both stages fail", regularErr: regularFailure, sourceErr: sourceFailure, wantError: true},
		{name: "stage and merge fail", regularErr: regularFailure, mergeErr: mergeFailure, wantError: true},
		{name: "merge alone fails", mergeErr: mergeFailure, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var ranStages []string
			mergeCalls := 0
			var mergeInput string
			err := runWithCleanup(t, context.Background(), nil, nil, func(_ context.Context, _ string, args, environ []string) error {
				if args[0] == "test" {
					stage := envValue(environ, "SKGO_E2E_STAGE")
					ranStages = append(ranStages, stage)
					if writeErr := os.WriteFile(envValue(environ, blobOutputFileEnv), []byte(stage), 0o600); writeErr != nil {
						t.Fatal(writeErr)
					}
					if stage == "regular" {
						return test.regularErr
					}
					return test.sourceErr
				}
				mergeCalls++
				mergeInput = args[1]
				if writeErr := os.WriteFile(filepath.Join(mergeInput, "report.jsonl"), []byte("merged events"), 0o600); writeErr != nil {
					t.Fatal(writeErr)
				}
				if writeErr := os.RemoveAll("test-results"); writeErr != nil {
					t.Fatal(writeErr)
				}
				if writeErr := os.MkdirAll("test-results", 0o755); writeErr != nil {
					t.Fatal(writeErr)
				}
				if writeErr := os.WriteFile(filepath.Join("test-results", "final-output"), []byte("reporter finished"), 0o600); writeErr != nil {
					t.Fatal(writeErr)
				}
				return test.mergeErr
			})
			if !reflect.DeepEqual(ranStages, []string{"regular", "source-edit"}) || mergeCalls != 1 {
				t.Fatalf("stages=%v mergeCalls=%d", ranStages, mergeCalls)
			}
			if (err != nil) != test.wantError {
				t.Fatalf("run error=%v wantError=%v", err, test.wantError)
			}
			archives, globErr := filepath.Glob(filepath.Join("test-results", "skgo-e2e-blobs-*"))
			if globErr != nil || len(archives) != 1 {
				t.Fatalf("post-merge archives=%v globErr=%v", archives, globErr)
			}
			if data, readErr := os.ReadFile(filepath.Join(archives[0], "report.jsonl")); readErr != nil || string(data) != "merged events" {
				t.Fatalf("archived native events=%q readErr=%v", data, readErr)
			}
			if data, readErr := os.ReadFile(filepath.Join("test-results", "final-output")); readErr != nil || string(data) != "reporter finished" {
				t.Fatalf("final reporter output=%q readErr=%v", data, readErr)
			}
			if data, readErr := os.ReadFile(filepath.Join(mergeInput, "report.jsonl")); readErr != nil || string(data) != "merged events" {
				t.Fatalf("native merge input did not persist: %q readErr=%v", data, readErr)
			}
			for _, want := range []error{test.regularErr, test.sourceErr, test.mergeErr} {
				if want != nil && !errors.Is(err, want) {
					t.Fatalf("aggregate error lost %v: %v", want, err)
				}
			}
		})
	}
}

func TestArchiveFailureKeepsStageAndMergeFailures(t *testing.T) {
	chdirTemp(t)
	regularFailure := errors.New("regular stage failed")
	mergeFailure := errors.New("native merge failed")
	var stagesRun []string
	err := runWithCleanup(t, context.Background(), nil, nil, func(_ context.Context, _ string, args, environ []string) error {
		if args[0] == "test" {
			stage := envValue(environ, "SKGO_E2E_STAGE")
			stagesRun = append(stagesRun, stage)
			if writeErr := os.WriteFile(envValue(environ, blobOutputFileEnv), []byte(stage), 0o600); writeErr != nil {
				t.Fatal(writeErr)
			}
			if stage == "regular" {
				return regularFailure
			}
			return nil
		}
		if writeErr := os.WriteFile("test-results", []byte("blocks archive parent"), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
		return mergeFailure
	})
	if !reflect.DeepEqual(stagesRun, []string{"regular", "source-edit"}) {
		t.Fatalf("stages=%v, want both ordered invocations", stagesRun)
	}
	if !errors.Is(err, regularFailure) || !errors.Is(err, mergeFailure) || !strings.Contains(err.Error(), "archive Playwright blob reports") {
		t.Fatalf("aggregate error=%v; want stage, native merge and archive failures", err)
	}
}

func TestArchiveFailureMakesSuccessfulNativeRunNonzero(t *testing.T) {
	chdirTemp(t)
	var stagesRun []string
	err := runWithCleanup(t, context.Background(), nil, nil, func(_ context.Context, _ string, args, environ []string) error {
		if args[0] == "test" {
			stage := envValue(environ, "SKGO_E2E_STAGE")
			stagesRun = append(stagesRun, stage)
			return os.WriteFile(envValue(environ, blobOutputFileEnv), []byte(stage), 0o600)
		}
		return os.WriteFile("test-results", []byte("blocks archive parent"), 0o600)
	})
	if !reflect.DeepEqual(stagesRun, []string{"regular", "source-edit"}) {
		t.Fatalf("stages=%v, want both ordered invocations", stagesRun)
	}
	if err == nil || !strings.Contains(err.Error(), "archive Playwright blob reports") {
		t.Fatalf("run error=%v; successful native reporting with failed archive must be nonzero", err)
	}
}

func TestMissingCurrentBlobDoesNotMergeStaleJsonFile(t *testing.T) {
	chdirTemp(t)
	stale := filepath.Join(t.TempDir(), "report.json")
	if err := os.WriteFile(stale, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	stageCalls, mergeCalls := 0, 0
	err := runWithCleanup(t, context.Background(), []string{"--reporter=json"}, []string{jsonOutputFileEnv + "=" + stale}, func(_ context.Context, _ string, args, environ []string) error {
		if args[0] == "test" {
			stageCalls++
			return nil // Native command did not emit its required fresh blob.
		}
		mergeCalls++
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "retain Playwright blob reports") || stageCalls != 2 || mergeCalls != 0 {
		t.Fatalf("err=%v stages=%d merges=%d", err, stageCalls, mergeCalls)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("known stale JSON FILE remains, stat error=%v", err)
	}
}

func TestKnownJSONNameIsClearedAtNativeDestination(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		env  func(cwd, configDir, absoluteName string) []string
		want string
	}{
		{
			name: "csv reporter contains exact json id and config file supplies directory",
			args: []string{"--reporter=dot,json", "--config", "external/playwright.config.js"},
			env: func(_, _, _ string) []string {
				return []string{jsonOutputEnv + "=report.json"}
			},
			want: "config",
		},
		{
			name: "DIR applies to relative NAME",
			args: []string{"--reporter=json"},
			env: func(_, _, _ string) []string {
				return []string{jsonOutputDirEnv + "=reports", jsonOutputEnv + "=report.json"}
			},
			want: "dir",
		},
		{
			name: "absolute NAME overrides DIR",
			args: []string{"--add-reporter=json"},
			env: func(_, _, absoluteName string) []string {
				return []string{jsonOutputDirEnv + "=ignored", jsonOutputEnv + "=" + absoluteName}
			},
			want: "absolute",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			cwd := chdirTemp(t)
			configDir := filepath.Join(cwd, "external")
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatal(err)
			}
			config := filepath.Join(configDir, "playwright.config.js")
			if err := os.WriteFile(config, []byte("export default {};\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			absoluteName := filepath.Join(t.TempDir(), "absolute-report.json")
			values := test.env(cwd, configDir, absoluteName)
			var target string
			switch test.want {
			case "config":
				target = filepath.Join(configDir, "report.json")
			case "dir":
				target = filepath.Join(cwd, "reports", "report.json")
			case "absolute":
				target = absoluteName
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("stale"), 0o600); err != nil {
				t.Fatal(err)
			}
			args := append([]string(nil), test.args...)
			for i := range args {
				if args[i] == "external/playwright.config.js" {
					args[i] = config
				}
			}
			stageCalls, mergeCalls := 0, 0
			err := runWithCleanup(t, context.Background(), args, values, func(_ context.Context, _ string, commandArgs, commandEnv []string) error {
				if commandArgs[0] == "test" {
					stageCalls++
					if stageCalls == 1 {
						if _, err := os.Stat(target); !os.IsNotExist(err) {
							t.Fatalf("stale known JSON NAME remains before stages, stat error=%v", err)
						}
					}
					return os.WriteFile(envValue(commandEnv, blobOutputFileEnv), []byte("blob"), 0o600)
				}
				mergeCalls++
				return nil
			})
			if err != nil || stageCalls != 2 || mergeCalls != 1 {
				t.Fatalf("run err=%v stages=%d merges=%d", err, stageCalls, mergeCalls)
			}
		})
	}
}

func TestKnownJSONDirectoryIsPreservedAndRejected(t *testing.T) {
	chdirTemp(t)
	destination := filepath.Join(t.TempDir(), "report.json")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	calls := 0
	err := runWithCleanup(t, context.Background(), []string{"--reporter=json"}, []string{jsonOutputFileEnv + "=" + destination}, func(context.Context, string, []string, []string) error {
		calls++
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "is a directory") || calls != 0 {
		t.Fatalf("directory destination err=%v calls=%d, want a pre-stage error", err, calls)
	}
	info, err := os.Stat(destination)
	if err != nil || !info.IsDir() {
		t.Fatalf("invalid JSON destination was removed: stat=%v info=%v", err, info)
	}
}

func TestOpaqueConfiguredJSONDestinationIsNotDeleted(t *testing.T) {
	chdirTemp(t)
	stale := filepath.Join(t.TempDir(), "configured.json")
	if err := os.WriteFile(stale, []byte("configured stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	stageCalls, mergeCalls := 0, 0
	err := runWithCleanup(t, context.Background(), nil, []string{jsonOutputFileEnv + "=" + stale}, func(_ context.Context, _ string, args, environ []string) error {
		if args[0] == "test" {
			stageCalls++
			return os.WriteFile(envValue(environ, blobOutputFileEnv), []byte("blob"), 0o600)
		}
		mergeCalls++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(stale)
	if err != nil || string(data) != "configured stale" || stageCalls != 2 || mergeCalls != 1 {
		t.Fatalf("opaque configured output was changed: data=%q err=%v stages=%d merges=%d", data, err, stageCalls, mergeCalls)
	}
}

func TestCancellationStopsFollowingStageAndMerge(t *testing.T) {
	chdirTemp(t)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := runWithCleanup(t, ctx, nil, nil, func(context.Context, string, []string, []string) error {
		calls++
		cancel()
		return errors.New("interrupted regular process")
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("err=%v calls=%d; want cancellation and no later stage/merge", err, calls)
	}
}

func writeBlobRunner(t *testing.T, after func(args, environ []string)) commandRunner {
	t.Helper()
	return func(_ context.Context, _ string, args, environ []string) error {
		if args[0] == "test" {
			if err := os.WriteFile(envValue(environ, blobOutputFileEnv), []byte(envValue(environ, "SKGO_E2E_STAGE")), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if after != nil {
			after(args, environ)
		}
		return nil
	}
}

func runWithCleanup(t *testing.T, ctx context.Context, args, environ []string, invoke commandRunner) error {
	t.Helper()
	return run(ctx, args, environ, func(ctx context.Context, name string, commandArgs, commandEnv []string) error {
		if len(commandArgs) > 1 && commandArgs[0] == "merge-reports" {
			mergeInput := commandArgs[1]
			t.Cleanup(func() { _ = os.RemoveAll(mergeInput) })
		}
		return invoke(ctx, name, commandArgs, commandEnv)
	})
}

func pathIsWithin(path, parent string) bool {
	relative, err := filepath.Rel(parent, path)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func chdirTemp(t *testing.T) string {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workingDir := t.TempDir()
	if err := os.Chdir(workingDir); err != nil {
		t.Fatal(err)
	}
	workingDir, err = os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	return workingDir
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	path, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func count(values []string, expected string) int {
	n := 0
	for _, value := range values {
		if value == expected {
			n++
		}
	}
	return n
}

func indexOf(values []string, expected string) int {
	for i, value := range values {
		if value == expected {
			return i
		}
	}
	return -1
}
