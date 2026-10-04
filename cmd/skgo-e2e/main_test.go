package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegularFailureStillRunsSourceEditAndCombinesReports(t *testing.T) {
	temp := t.TempDir()
	output := filepath.Join(temp, "playwright-dev.json")
	regularFailure := errors.New("regular stage failed")
	var calls []string
	err := run(context.Background(), []string{"--reporter=json"}, []string{
		jsonOutputFileEnv + "=" + output,
		jsonOutputDirEnv + "=" + filepath.Join(temp, "ignored"),
		jsonOutputEnv + "=ignored.json",
		"SKGO_E2E_RUN=proof",
	}, func(_ context.Context, name string, args, environ []string) error {
		if name != "./node_modules/.bin/playwright" {
			t.Fatalf("command = %q, want the local Playwright binary", name)
		}
		stageName := envValue(environ, "SKGO_E2E_STAGE")
		calls = append(calls, stageName)
		if got := envValue(environ, "SKGO_E2E_RUN"); got != "proof-"+stageName {
			t.Fatalf("SKGO_E2E_RUN = %q for %s", got, stageName)
		}
		if !contains(args, "--reporter=json") {
			t.Fatalf("Playwright arguments lost external JSON reporter: %v", args)
		}
		if !contains(args, "--project=chromium") && stageName == "regular" {
			t.Fatalf("regular project missing: %v", args)
		}
		if !contains(args, "--project=source-edit") && stageName == "source-edit" {
			t.Fatalf("source-edit project missing: %v", args)
		}
		stageOutput := envValue(environ, jsonOutputFileEnv)
		if stageOutput == "" || stageOutput == output || !filepath.IsAbs(stageOutput) {
			t.Fatalf("stage JSON file = %q, want a distinct absolute path", stageOutput)
		}
		if envValue(environ, jsonOutputDirEnv) != "" || envValue(environ, jsonOutputEnv) != "" {
			t.Fatalf("caller JSON directory/name leaked into %s stage: %v", stageName, environ)
		}
		report := map[string]any{
			"config": map[string]any{"workers": 6},
			"suites": []any{map[string]any{"title": stageName}},
			"errors": []any{},
			"stats":  map[string]any{"expected": 1, "unexpected": 0, "duration": 4.0},
		}
		if stageName == "regular" {
			report["stats"] = map[string]any{"expected": 2, "unexpected": 1, "duration": 10.0}
		}
		data, marshalErr := json.Marshal(report)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if writeErr := os.WriteFile(stageOutput, data, 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
		if stageName == "regular" {
			return regularFailure
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "regular Playwright stage failed") {
		t.Fatalf("run error = %v, want regular failure", err)
	}
	if strings.Join(calls, ",") != "regular,source-edit" {
		t.Fatalf("stage calls = %v, want regular then source-edit", calls)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("combined report missing at caller path: %v", err)
	}
	var report struct {
		Config map[string]any `json:"config"`
		Suites []struct {
			Title string `json:"title"`
		} `json:"suites"`
		Stats map[string]float64 `json:"stats"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("combined report is not JSON: %v", err)
	}
	if len(report.Suites) != 2 || report.Suites[0].Title != "regular" || report.Suites[1].Title != "source-edit" {
		t.Fatalf("report suites = %+v, want both stages in order", report.Suites)
	}
	if report.Config["workers"] != float64(6) || report.Stats["expected"] != 3 || report.Stats["unexpected"] != 1 {
		t.Fatalf("combined report lost config or failure stats: %+v", report)
	}
	if report.Stats["duration"] != 14 {
		t.Fatalf("combined duration = %v, want 14", report.Stats["duration"])
	}
	for _, stageName := range []string{"regular", "source-edit"} {
		if _, err := os.Stat(stageReportPath(output, stageName, os.Getpid())); !os.IsNotExist(err) {
			t.Fatalf("temporary %s report should be folded into combined report, stat error = %v", stageName, err)
		}
	}
}

func TestNativeJSONDestinationsFollowPlaywrightResolution(t *testing.T) {
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	temp := t.TempDir()
	configDir := filepath.Join(temp, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(configDir, "playwright.config.js")
	if err := os.WriteFile(configFile, []byte("export default {};\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	relativeOutputDir, err := os.MkdirTemp(".", ".skgo-json-output-dir-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(relativeOutputDir)

	absoluteName := filepath.Join(temp, "absolute-name.json")
	for _, test := range []struct {
		name string
		args []string
		env  []string
		want string
	}{
		{
			name: "output file takes precedence",
			args: []string{"--reporter=json"},
			env: []string{
				jsonOutputFileEnv + "=" + filepath.Join(relativeOutputDir, "file.json"),
				jsonOutputDirEnv + "=" + filepath.Join(temp, "ignored"),
				jsonOutputEnv + "=ignored.json",
			},
			want: filepath.Join(workingDir, relativeOutputDir, "file.json"),
		},
		{
			name: "relative name uses relative output directory from working directory",
			args: []string{"--reporter=json"},
			env: []string{
				jsonOutputDirEnv + "=" + relativeOutputDir,
				jsonOutputEnv + "=directory-name.json",
			},
			want: filepath.Join(workingDir, relativeOutputDir, "directory-name.json"),
		},
		{
			name: "relative name without output directory uses config directory",
			args: []string{"--config", configFile, "--reporter=json"},
			env:  []string{jsonOutputEnv + "=config-name.json"},
			want: filepath.Join(configDir, "config-name.json"),
		},
		{
			name: "absolute name overrides output directory",
			args: []string{"--reporter=json"},
			env: []string{
				jsonOutputDirEnv + "=" + filepath.Join(temp, "ignored"),
				jsonOutputEnv + "=" + absoluteName,
			},
			want: absoluteName,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			err := run(context.Background(), test.args, test.env, func(_ context.Context, _ string, args, environ []string) error {
				calls++
				if !contains(args, "--reporter=json") {
					t.Fatalf("native JSON destination must activate the reporter: %v", args)
				}
				stagePath := envValue(environ, jsonOutputFileEnv)
				if stagePath == "" || stagePath == test.want || !filepath.IsAbs(stagePath) {
					t.Fatalf("stage JSON file = %q, want a distinct absolute path from %q", stagePath, test.want)
				}
				if envValue(environ, jsonOutputDirEnv) != "" || envValue(environ, jsonOutputEnv) != "" {
					t.Fatalf("caller JSON destination leaked into stage environment: %v", environ)
				}
				data, err := json.Marshal(map[string]any{
					"suites": []any{map[string]any{"title": envValue(environ, "SKGO_E2E_STAGE")}},
					"errors": []any{},
					"stats":  map[string]any{"expected": 1, "unexpected": 0},
				})
				if err != nil {
					t.Fatal(err)
				}
				return os.WriteFile(stagePath, data, 0o600)
			})
			if err != nil {
				t.Fatalf("run failed: %v", err)
			}
			if calls != 2 {
				t.Fatalf("invocation count = %d, want both stages", calls)
			}
			data, err := os.ReadFile(test.want)
			if err != nil {
				t.Fatalf("combined report missing at native destination %q: %v", test.want, err)
			}
			var report struct {
				Suites []struct {
					Title string `json:"title"`
				} `json:"suites"`
				Stats map[string]float64 `json:"stats"`
			}
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatalf("combined report is not JSON: %v", err)
			}
			if len(report.Suites) != 2 || report.Suites[0].Title != "regular" || report.Suites[1].Title != "source-edit" {
				t.Fatalf("combined suites = %+v, want both stages in order", report.Suites)
			}
			if report.Stats["expected"] != 2 || report.Stats["unexpected"] != 0 {
				t.Fatalf("combined stats = %+v, want both stage results", report.Stats)
			}
		})
	}
}

func TestStageFailuresRemainAggregateAndSuccessPasses(t *testing.T) {
	regularFailure := errors.New("regular")
	sourceFailure := errors.New("source")
	for _, test := range []struct {
		name       string
		regularErr error
		sourceErr  error
		wantError  bool
	}{
		{name: "both succeed"},
		{name: "regular fails", regularErr: regularFailure, wantError: true},
		{name: "source-edit fails", sourceErr: sourceFailure, wantError: true},
		{name: "both fail", regularErr: regularFailure, sourceErr: sourceFailure, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := 0
			err := run(context.Background(), nil, nil, func(context.Context, string, []string, []string) error {
				call++
				if call == 1 {
					return test.regularErr
				}
				return test.sourceErr
			})
			if call != 2 {
				t.Fatalf("invocation count = %d, want both stages", call)
			}
			if (err != nil) != test.wantError {
				t.Fatalf("run error = %v, wantError=%v", err, test.wantError)
			}
			if test.regularErr != nil && !errors.Is(err, test.regularErr) {
				t.Fatalf("aggregate error lost regular failure: %v", err)
			}
			if test.sourceErr != nil && !errors.Is(err, test.sourceErr) {
				t.Fatalf("aggregate error lost source-edit failure: %v", err)
			}
		})
	}
}

func TestInterruptStopsBeforeSourceEdit(t *testing.T) {
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
		t.Fatalf("invocation count = %d, want source-edit stopped after interrupt", calls)
	}
}

func TestMissingStageReportDoesNotReusePriorCombinedJSON(t *testing.T) {
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
	calls := 0
	err := run(context.Background(), nil, []string{
		jsonOutputFileEnv + "=" + output,
		jsonOutputDirEnv + "=" + filepath.Dir(ignoredOutput),
		jsonOutputEnv + "=" + filepath.Base(ignoredOutput),
	}, func(_ context.Context, _ string, args, environ []string) error {
		calls++
		if !contains(args, "--reporter=json") {
			t.Fatalf("JSON output path must activate the reporter: %v", args)
		}
		if envValue(environ, jsonOutputFileEnv) == "" || envValue(environ, jsonOutputFileEnv) == output {
			t.Fatal("stage must write to its own JSON file path")
		}
		if envValue(environ, jsonOutputDirEnv) != "" || envValue(environ, jsonOutputEnv) != "" {
			t.Fatalf("caller JSON destination leaked into stage environment: %v", environ)
		}
		return nil // Simulate a successful command that failed to produce its report.
	})
	if err == nil || !strings.Contains(err.Error(), "combine Playwright JSON reports") {
		t.Fatalf("run error = %v, want missing stage report error", err)
	}
	if calls != 2 {
		t.Fatalf("invocation count = %d, want both stages", calls)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("stale combined report remains, stat error = %v", err)
	}
	if _, err := os.Stat(ignoredOutput); err != nil {
		t.Fatalf("ignored output name should not be treated as the native destination: %v", err)
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

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
