package wirejson

import (
	"github.com/tylergannon/devalue/v5"
	"testing"
)

func TestArguments(t *testing.T) {
	for _, raw := range []string{`{"a":1,"a":2}`, `{} {}`, `{"a":9007199254740993}`, `null`, `[]`, `{"a":1e999}`} {
		if _, err := Arguments([]byte(raw), true, false); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	value, err := Arguments([]byte(`{"title":"Ship MCP"}`), true, false)
	if err != nil {
		t.Fatal(err)
	}
	if title, _ := value.(*devalue.Object).Get("title"); title != "Ship MCP" {
		t.Fatalf("title=%v", title)
	}
	value, err = Arguments([]byte(`{}`), false, true)
	if err != nil || value != devalue.Undefined {
		t.Fatalf("no input=%v %v", value, err)
	}
	value, err = Arguments([]byte(`{"argument":null}`), false, false)
	if err != nil || value != nil {
		t.Fatalf("null=%v %v", value, err)
	}
	if _, err := Arguments([]byte(`{"surprise":true}`), false, true); err == nil {
		t.Fatal("accepted unexpected argument")
	}
}

func TestJSONResult(t *testing.T) {
	value, err := Value(devalue.NewObject("id", float64(7), "titles", []any{"Ship MCP"}))
	if err != nil || value.(map[string]any)["id"] != float64(7) {
		t.Fatalf("result=%v %v", value, err)
	}
	if _, err := Value(float64(9007199254740992)); err == nil {
		t.Fatal("accepted unsafe integer")
	}
	if _, err := Value(devalue.Undefined); err == nil {
		t.Fatal("accepted undefined")
	}
}
