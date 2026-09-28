package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentCommandValidatesBuildInputsInGo(t *testing.T) {
	schema := filepath.Join(t.TempDir(), "env.json")
	if err := os.WriteFile(schema, []byte(`{"fields":[{"name":"ID","field":"ID","type":"string"},{"name":"COUNT","field":"Count","type":"int"},{"name":"ENABLED","field":"Enabled","type":"bool"},{"name":"OPTIONAL","field":"Optional","type":"bool","optional":true}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := environmentCommand([]string{"--schema", schema}, strings.NewReader(`{"ID":"123","COUNT":"123","ENABLED":"yes"}`), &out)
	if err != nil {
		t.Fatal(err)
	}
	if out.String() != "{\"COUNT\":123,\"ENABLED\":true,\"ID\":\"123\"}\n" {
		t.Fatalf("unexpected typed build values: %s", out.String())
	}
	out.Reset()
	err = environmentCommand([]string{"--schema", schema}, strings.NewReader(`{"ID":"123","COUNT":"123","ENABLED":"secret-invalid-value"}`), &out)
	if err == nil || !strings.Contains(err.Error(), "ENABLED") || strings.Contains(err.Error(), "secret-invalid-value") || out.Len() != 0 {
		t.Fatalf("invalid build input not safely refused: err=%v stdout=%q", err, out.String())
	}
	out.Reset()
	err = environmentCommand([]string{"--schema", schema}, strings.NewReader(`{"ID":"123","COUNT":"123"}`), &out)
	if err == nil || !strings.Contains(err.Error(), "ENABLED") || out.Len() != 0 {
		t.Fatalf("required dynamic build input silently optional: err=%v stdout=%q", err, out.String())
	}
}
