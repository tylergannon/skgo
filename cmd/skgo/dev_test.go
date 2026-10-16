package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestDevHelpDoesNotStartApplication(t *testing.T) {
	var out bytes.Buffer
	if e := devCommand(context.Background(), []string{"--help"}, &out, &out); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "-listen") || !strings.Contains(usage, "skgo dev") {
		t.Fatalf("development CLI help missing: %s", out.String())
	}
}
