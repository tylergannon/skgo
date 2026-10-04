package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegularFailureStillMergesBothNativeBlobReports(t *testing.T) {
	workingDir := chdirTemp(t)
	output := filepath.Join(t.TempDir(), "playwright.json")
	callerBlobFile := filepath.Join(t.TempDir(), "caller-blob.zip")
	if err := os.WriteFile(output, []byte(`{"stale":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	regularFailure := errors.New("regular stage failed")
	var calls []string
	var retainedDir string
	err := run(context.Background(), []string{"--reporter=json", "--grep", "literal", "--project=source-edit"}, []string{
		jsonOutputFileEnv + "=" + output,
		jsonOutputDirEnv + "=" + filepath.Join(t.TempDir(), "ignored"),
		jsonOutputEnv + "=ignored.json",
		blobOutputFileEnv + "=" + callerBlobFile,
		blobOutputDirEnv + "=" + filepath.Join(t.TempDir(), "caller-blob-dir"),
		blobOutputEnv + "=caller-blob.zip",
		"SKGO_E2E_RUN=proof",
	}, func(_ context.Context, name string, args, environ []string) error {
		if name != "./node_modules/.bin/playwright" {
			t.Fatalf("command = %q, want the local Playwright binary", name)
		}
		if len(args) == 0 {
			t.Fatal("Playwright subcommand is missing")
		}
		calls = append(calls, args[0])
		switch args[0] {
		case "test":
			stageName := envValue(environ, "SKGO_E2E_STAGE")
			if got := envValue(environ, "SKGO_E2E_RUN"); got != "proof-"+stageName {
				t.Fatalf("SKGO_E2E_RUN = %q for %s", got, stageName)
			}
			if !contains(args, "--reporter=blob") || hasJSONReporter(args) {
				t.Fatalf("stage must use only the owned blob reporter: %v", args)
			}
			if !contains(args, "--grep") || !contains(args, "literal") {
				t.Fatalf("test filter was not preserved: %v", args)
			}
			if stageName == "regular" && contains(args, "--project=source-edit") {
				t.Fatalf("caller project selection must not expand the mutation stage: %v", args)
			}
			if stageName == "regular" && (!contains(args, "--project=chromium") || !contains(args, "--project=noscript")) {
				t.Fatalf("regular projects missing: %v", args)
			}
			if stageName == "source-edit" && !contains(args, "--project=source-edit") {
				t.Fatalf("source-edit project missing: %v", args)
			}
			stageBlob := envValue(environ, blobOutputFileEnv)
			if stageBlob == "" || !filepath.IsAbs(stageBlob) || stageBlob == callerBlobFile {
				t.Fatalf("stage blob destination = %q, want a private absolute path", stageBlob)
			}
			if envValue(environ, blobOutputDirEnv) != filepath.Dir(stageBlob) {
				t.Fatalf("stage blob directory = %q, want %q", envValue(environ, blobOutputDirEnv), filepath.Dir(stageBlob))
			}
			for _, key := range jsonOutputEnvs {
				if value := envValue(environ, key); value != "" {
					t.Fatalf("caller JSON destination %s leaked into stage: %q", key, value)
				}
			}
			if envValue(environ, blobOutputEnv) != "" {
				t.Fatalf("caller blob name leaked into stage: %q", envValue(environ, blobOutputEnv))
			}
			if envValue(environ, blobOutputFileEnv) == callerBlobFile {
				t.Fatalf("caller blob file leaked into stage: %q", envValue(environ, blobOutputFileEnv))
			}
			if err := os.WriteFile(stageBlob, []byte("blob-"+stageName), 0o600); err != nil {
				t.Fatal(err)
			}
			if stageName == "regular" {
				return regularFailure
			}
			return nil
		case "merge-reports":
			if len(args) < 2 {
				t.Fatalf("merge command lacks blob directory: %v", args)
			}
			retainedDir = args[1]
			if !filepath.IsAbs(retainedDir) || !strings.Contains(retainedDir, string(filepath.Join("test-results", "skgo-e2e-blobs-"))) {
				t.Fatalf("merged blob directory = %q, want retained test-results directory", retainedDir)
			}
			if got := envValue(environ, jsonOutputFileEnv); got != output {
				t.Fatalf("merge JSON output env = %q, want caller destination %q", got, output)
			}
			if envValue(environ, jsonOutputDirEnv) == "" || envValue(environ, jsonOutputEnv) != "ignored.json" {
				t.Fatalf("final merge lost caller reporter environment: %v", environ)
			}
			if envValue(environ, "SKGO_E2E_RUN") != "proof" || envValue(environ, "SKGO_E2E_STAGE") != "" {
				t.Fatalf("merge environment should be caller-owned, not stage-owned: %v", environ)
			}
			if got := envValue(environ, blobOutputFileEnv); got != callerBlobFile {
				t.Fatalf("final merge blob env = %q, want caller value %q", got, callerBlobFile)
			}
			if !contains(args, "--config") || !contains(args, workingDir) || !contains(args, "--reporter") || !contains(args, "json") {
				t.Fatalf("merge did not preserve cwd config and JSON selection: %v", args)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("stale JSON report should be removed before merge, stat error = %v", err)
			}
			for _, stageName := range []string{"regular", "source-edit"} {
				blob, err := os.ReadFile(filepath.Join(retainedDir, stageName+".zip"))
				if err != nil || string(blob) != "blob-"+stageName {
					t.Fatalf("retained %s blob = %q, err=%v", stageName, blob, err)
				}
			}
			if err := os.WriteFile(output, []byte(`{"merged":true}`), 0o600); err != nil {
				t.Fatal(err)
			}
			return nil
		default:
			t.Fatalf("unexpected Playwright command: %v", args)
			return nil
		}
	})
	if err == nil || !strings.Contains(err.Error(), "regular Playwright stage failed") || !errors.Is(err, regularFailure) {
		t.Fatalf("run error = %v, want retained regular stage failure", err)
	}
	if strings.Join(calls, ",") != "test,test,merge-reports" {
		t.Fatalf("Playwright commands = %v, want both stages then native merge", calls)
	}
	if retainedDir != "" {
		t.Cleanup(func() { _ = os.RemoveAll(retainedDir) })
	}
	data, err := os.ReadFile(output)
	if err != nil || string(data) != `{"merged":true}` {
		t.Fatalf("native merged output = %q, err=%v", data, err)
	}
}

func TestNativeJSONDestinationsAreUsedOnlyByFinalMerge(t *testing.T) {
	workingDir := chdirTemp(t)
	temp := t.TempDir()
	configDir := filepath.Join(temp, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(configDir, "playwright.config.js")
	if err := os.WriteFile(configFile, []byte("export default {};\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	relativeOutputDir, err := os.MkdirTemp(workingDir, ".skgo-json-output-dir-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(relativeOutputDir) })
	absoluteName := filepath.Join(temp, "absolute-name.json")
	for _, test := range []struct {
		name string
		args []string
		env  []string
		want string
	}{
		{
			name: "absolute output file takes precedence",
			args: nil,
			env: []string{
				jsonOutputFileEnv + "=" + filepath.Join(relativeOutputDir, "file.json"),
				jsonOutputDirEnv + "=" + filepath.Join(temp, "ignored"),
				jsonOutputEnv + "=ignored.json",
			},
			want: filepath.Join(relativeOutputDir, "file.json"),
		},
		{
			name: "relative output file resolves from working directory",
			env:  []string{jsonOutputFileEnv + "=" + filepath.Join(filepath.Base(relativeOutputDir), "relative-file.json")},
			want: filepath.Join(relativeOutputDir, "relative-file.json"),
		},
		{
			name: "relative name uses relative output directory from working directory",
			env: []string{
				jsonOutputDirEnv + "=" + filepath.Base(relativeOutputDir),
				jsonOutputEnv + "=directory-name.json",
			},
			want: filepath.Join(relativeOutputDir, "directory-name.json"),
		},
		{
			name: "relative name without output directory uses config directory",
			args: []string{"--config", configFile},
			env:  []string{jsonOutputEnv + "=config-name.json"},
			want: filepath.Join(configDir, "config-name.json"),
		},
		{
			name: "attached short config value uses config directory",
			args: []string{"-c" + configFile},
			env:  []string{jsonOutputEnv + "=attached-short-name.json"},
			want: filepath.Join(configDir, "attached-short-name.json"),
		},
		{
			name: "separated short config value uses config directory",
			args: []string{"-c", configFile},
			env:  []string{jsonOutputEnv + "=separated-short-name.json"},
			want: filepath.Join(configDir, "separated-short-name.json"),
		},
		{
			name: "long config equals value uses config directory",
			args: []string{"--config=" + configFile},
			env:  []string{jsonOutputEnv + "=equals-long-name.json"},
			want: filepath.Join(configDir, "equals-long-name.json"),
		},
		{
			name: "absolute name overrides output directory",
			env: []string{
				jsonOutputDirEnv + "=" + filepath.Join(temp, "ignored"),
				jsonOutputEnv + "=" + absoluteName,
			},
			want: absoluteName,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.MkdirAll(filepath.Dir(test.want), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(test.want, []byte(`{"old":true}`), 0o600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			var retainedDir string
			err := run(context.Background(), append(test.args, "--grep", "literal"), test.env, func(_ context.Context, _ string, args, environ []string) error {
				calls++
				if args[0] == "test" {
					if !contains(args, "--reporter=blob") || !contains(args, "--grep") || !contains(args, "literal") {
						t.Fatalf("stage flags = %v, want blob plus preserved test selection", args)
					}
					if !filepath.IsAbs(envValue(environ, blobOutputFileEnv)) || envValue(environ, blobOutputDirEnv) == "" {
						t.Fatalf("stage blob destination is not private: %v", environ)
					}
					for _, key := range jsonOutputEnvs {
						if envValue(environ, key) != "" {
							t.Fatalf("caller JSON output %s leaked to stage: %v", key, environ)
						}
					}
					return os.WriteFile(envValue(environ, blobOutputFileEnv), []byte("blob"), 0o600)
				}
				if args[0] != "merge-reports" {
					t.Fatalf("unexpected command: %v", args)
				}
				retainedDir = args[1]
				for _, entry := range test.env {
					key, value, ok := strings.Cut(entry, "=")
					if ok && envValue(environ, key) != value {
						t.Fatalf("final merge %s = %q, want caller value %q", key, envValue(environ, key), value)
					}
				}
				if !contains(args, "--reporter") || !contains(args, "json") {
					t.Fatalf("env-only destination must select native JSON reporter on merge: %v", args)
				}
				if _, err := os.Stat(test.want); !os.IsNotExist(err) {
					t.Fatalf("stale native report remains before merge, stat error = %v", err)
				}
				if err := os.WriteFile(test.want, []byte(`{"fresh":true}`), 0o600); err != nil {
					t.Fatal(err)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("run failed: %v", err)
			}
			if calls != 3 {
				t.Fatalf("Playwright invocation count = %d, want two stages and one merge", calls)
			}
			if retainedDir != "" {
				t.Cleanup(func() { _ = os.RemoveAll(retainedDir) })
			}
			data, err := os.ReadFile(test.want)
			if err != nil || string(data) != `{"fresh":true}` {
				t.Fatalf("native report at %q = %q, err=%v", test.want, data, err)
			}
			mergeArgs := mergeConfigArgs(append(test.args, "--grep", "literal"), workingDir)
			if !contains(mergeArgs, workingDir) && !contains(mergeArgs, configFile) {
				t.Fatalf("merge config selection not preserved: %v", mergeArgs)
			}
		})
	}
}

func TestStageAndMergeFailuresRemainAggregate(t *testing.T) {
	chdirTemp(t)
	regularFailure := errors.New("regular")
	sourceFailure := errors.New("source-edit")
	mergeFailure := errors.New("native merge failed")
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
	} {
		t.Run(test.name, func(t *testing.T) {
			var stagesRun []string
			mergeCalls := 0
			got := run(context.Background(), nil, nil, func(_ context.Context, _ string, args, environ []string) error {
				if args[0] == "test" {
					stageName := envValue(environ, "SKGO_E2E_STAGE")
					stagesRun = append(stagesRun, stageName)
					if err := os.WriteFile(envValue(environ, blobOutputFileEnv), []byte("blob-"+stageName), 0o600); err != nil {
						t.Fatal(err)
					}
					if stageName == "regular" {
						return test.regularErr
					}
					return test.sourceErr
				}
				mergeCalls++
				if args[0] != "merge-reports" {
					t.Fatalf("unexpected command: %v", args)
				}
				return test.mergeErr
			})
			if strings.Join(stagesRun, ",") != "regular,source-edit" || mergeCalls != 1 {
				t.Fatalf("stages=%v mergeCalls=%d, want both stages and one merge", stagesRun, mergeCalls)
			}
			if (got != nil) != test.wantError {
				t.Fatalf("run error = %v, wantError=%v", got, test.wantError)
			}
			for _, want := range []error{test.regularErr, test.sourceErr, test.mergeErr} {
				if want != nil && !errors.Is(got, want) {
					t.Fatalf("aggregate error lost %v: %v", want, got)
				}
			}
		})
	}
}

func TestInterruptStopsBeforeSourceEditOrMerge(t *testing.T) {
	chdirTemp(t)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := run(ctx, nil, nil, func(context.Context, string, []string, []string) error {
		calls++
		cancel()
		return errors.New("interrupted regular process")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run error = %v, want context cancellation", err)
	}
	if calls != 1 {
		t.Fatalf("Playwright invocation count = %d, want source-edit and merge stopped after interrupt", calls)
	}
}

func TestMissingStageBlobDoesNotReusePriorEnvironmentJSON(t *testing.T) {
	chdirTemp(t)
	output := filepath.Join(t.TempDir(), "playwright.json")
	if err := os.WriteFile(output, []byte(`{"stale":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ignoredOutput := filepath.Join(t.TempDir(), "ignored", "name.json")
	if err := os.MkdirAll(filepath.Dir(ignoredOutput), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignoredOutput, []byte(`{"ignored-stale":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stageCalls, mergeCalls := 0, 0
	err := run(context.Background(), nil, []string{
		jsonOutputFileEnv + "=" + output,
		jsonOutputDirEnv + "=" + filepath.Dir(ignoredOutput),
		jsonOutputEnv + "=" + filepath.Base(ignoredOutput),
	}, func(_ context.Context, _ string, args, _ []string) error {
		if args[0] == "test" {
			stageCalls++
			return nil // Simulate Playwright exiting without producing a blob.
		}
		mergeCalls++
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "retain Playwright blob reports") {
		t.Fatalf("run error = %v, want missing blob report error", err)
	}
	if stageCalls != 2 || mergeCalls != 0 {
		t.Fatalf("stageCalls=%d mergeCalls=%d, want both stages and no incomplete merge", stageCalls, mergeCalls)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("stale environment JSON remains, stat error = %v", err)
	}
	if _, err := os.Stat(ignoredOutput); err != nil {
		t.Fatalf("lower-precedence caller output should not be treated as destination: %v", err)
	}
}

func TestReporterSelectionIsOwnedByMergeAndStageBlob(t *testing.T) {
	for _, test := range []struct {
		args       []string
		wantSelect string
		wantFound  bool
		wantStage  string
	}{
		{args: []string{"--reporter=json"}, wantSelect: "json", wantFound: true, wantStage: "--reporter=blob"},
		{args: []string{"--reporter", "list,json"}, wantSelect: "list,json", wantFound: true, wantStage: "--reporter=blob"},
		{args: []string{"-r=dot"}, wantSelect: "dot", wantFound: true, wantStage: "--reporter=blob"},
		{args: []string{"--", "fixture.spec.js"}, wantFound: false, wantStage: "--reporter=blob"},
	} {
		got, found := reporterSelection(test.args)
		if got != test.wantSelect || found != test.wantFound {
			t.Fatalf("reporterSelection(%v) = %q, %v", test.args, got, found)
		}
		stageArgs := testArgsWithReporter(test.args, "blob")
		if !contains(stageArgs, test.wantStage) {
			t.Fatalf("stage arguments = %v, want owned blob reporter", stageArgs)
		}
		if test.wantFound && hasJSONReporter(stageArgs) {
			t.Fatalf("original reporter leaked into stage: %v", stageArgs)
		}
	}
}

func TestProjectSelectionCannotExpandMutationStage(t *testing.T) {
	got, err := withoutProjectSelection([]string{"--", "--project=source-edit", "--grep", "route", "--project", "chromium"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != "--grep route" {
		t.Fatalf("arguments = %v, want only non-project filters forwarded", got)
	}
	if _, err := withoutProjectSelection([]string{"--project"}); err == nil {
		t.Fatal("missing project argument should fail")
	}
}

func TestConfigOptionFormsArePassedToNativeMerge(t *testing.T) {
	workingDir := filepath.Join(string(filepath.Separator), "work")
	for _, args := range [][]string{
		{"-c/path/config.js"},
		{"-c", "/path/config.js"},
		{"--config", "/path/config.js"},
		{"--config=/path/config.js"},
	} {
		got := mergeConfigArgs(args, workingDir)
		if strings.Join(got, " ") != "--config /path/config.js" {
			t.Fatalf("mergeConfigArgs(%v) = %v", args, got)
		}
	}
	if got := mergeConfigArgs(nil, workingDir); strings.Join(got, " ") != "--config "+workingDir {
		t.Fatalf("default merge config args = %v, want cwd discovery", got)
	}
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

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
