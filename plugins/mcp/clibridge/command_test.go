package clibridge

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tylergannon/devalue/v5"
	"github.com/tylergannon/skgo/plugins/mcp/wirejson"
)

func TestStringInputsAndFailureBeforeDispatch(t *testing.T) {
	file := filepath.Join(t.TempDir(), "task.txt")
	const contents = "@literal from file\nsecond line\n"
	if err := os.WriteFile(file, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, input, want string
		fail              bool
	}{
		{name: "literal", input: "Ship CLI", want: "Ship CLI"},
		{name: "file", input: "@" + file, want: contents},
		{name: "escape", input: "@@owner", want: "@owner"},
		{name: "one pass", input: "@@@owner", want: "@@owner"},
		{name: "missing", input: "@" + file + ".missing", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			cmd := Command(titleSpec(), func(_ context.Context, arg any) (any, error) {
				calls++
				title, _ := arg.(*devalue.Object).Get("title")
				if title != tc.want {
					t.Fatalf("title=%q want=%q", title, tc.want)
				}
				return devalue.NewObject("title", title), nil
			})
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetArgs([]string{"--title", tc.input})
			err := cmd.Execute()
			if tc.fail {
				if err == nil || !strings.Contains(err.Error(), "use @@") || calls != 0 {
					t.Fatalf("calls=%d err=%v", calls, err)
				}
			} else if err != nil || calls != 1 || !strings.Contains(out.String(), `"result":`) {
				t.Fatalf("calls=%d err=%v out=%s", calls, err, &out)
			}
		})
	}
}

func titleSpec() Spec {
	return Spec{Name: "add", Description: "Create a task.", InputFlag: "input-json", ObjectInput: true, Mutation: true, Parameters: []Parameter{{Name: "title", Flag: "title", Kind: "string", Required: true, Description: "The task title."}}}
}

func TestJSONInputIsLiteralAndHelpDoesNotCallApplication(t *testing.T) {
	for _, tc := range []struct {
		args      []string
		wantCalls int
		wantError string
	}{
		{args: []string{"--input-json", `{"title":"@not-a-file"}`}, wantCalls: 1},
		{args: []string{"--input-json", `{"title":"one","title":"two"}`}, wantError: "duplicate"},
		{args: []string{"--input-json", `{"title":"one"}`, "--title", "two"}, wantError: "cannot be combined"},
		{args: []string{}, wantError: "required flag --title"},
		{args: []string{"--help"}},
	} {
		calls := 0
		cmd := Command(titleSpec(), func(_ context.Context, arg any) (any, error) {
			calls++
			title, _ := arg.(*devalue.Object).Get("title")
			if title != "@not-a-file" {
				t.Fatalf("title=%v", title)
			}
			return title, nil
		})
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetArgs(tc.args)
		err := cmd.Execute()
		if tc.wantError != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("args=%v err=%v", tc.args, err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
		if calls != tc.wantCalls {
			t.Fatalf("calls=%d", calls)
		}
		if len(tc.args) == 1 && tc.args[0] == "--help" && (!strings.Contains(out.String(), "The task title.") || !strings.Contains(out.String(), "@@text")) {
			t.Fatal(out.String())
		}
	}
}

func TestBooleanStructuredAndOptionalInputs(t *testing.T) {
	spec := Spec{Name: "update", ObjectInput: true, InputFlag: "input-json", Parameters: []Parameter{{Name: "enabled", Flag: "enabled", Kind: "bool", Required: true}, {Name: "tags", Flag: "tags", Kind: "json", Required: true}, {Name: "note", Flag: "note", Kind: "string"}}}
	cmd := Command(spec, func(_ context.Context, arg any) (any, error) {
		obj := arg.(*devalue.Object)
		enabled, _ := obj.Get("enabled")
		if enabled != false {
			t.Fatalf("enabled=%v", enabled)
		}
		tags, _ := obj.Get("tags")
		if tags.([]any)[0] != "alpha" {
			t.Fatalf("tags=%v", tags)
		}
		if _, exists := obj.Get("note"); exists {
			t.Fatal("absent optional field became present")
		}
		return "ok", nil
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--enabled=false", "--tags", `["alpha"]`})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.String() != `{"result":"ok"}`+"\n" {
		t.Fatal(out.String())
	}
}

func TestMutationErrorDoesNotRetryAndDistinguishesRejectedInput(t *testing.T) {
	for _, err := range []error{errors.New("lost response"), wirejson.ErrInvalidInput} {
		calls := 0
		cmd := Command(Spec{Name: "write", NoInput: true, Mutation: true}, func(context.Context, any) (any, error) { calls++; return nil, err })
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetArgs([]string{})
		got := cmd.Execute()
		if calls != 1 || !errors.Is(got, err) {
			t.Fatalf("calls=%d err=%v", calls, got)
		}
		if strings.Contains(got.Error(), "may have taken effect") == errors.Is(err, wirejson.ErrInvalidInput) {
			t.Fatal(got)
		}
	}
}
