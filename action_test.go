package skgo

import (
	"context"
	"strings"
	"testing"
)

func TestNewActionsEnforcesKitDeclarationRules(t *testing.T) {
	const page = "src/routes/profile/+page.server.ts"
	run := func(context.Context) (any, error) { return nil, nil }
	action := func(module, name string) *PageAction {
		return NewPageAction(ActionSpec{Module: module, Name: name, Run: run})
	}
	tests := []struct {
		name    string
		actions []*PageAction
		want    string
	}{
		{"named actions", []*PageAction{action(page, "save"), action(page, "archive")}, ""},
		{"default only", []*PageAction{action(page, "default")}, ""},
		{"same names on different pages", []*PageAction{action(page, "save"), action("src/routes/other/+page.server.ts", "save")}, ""},
		{"duplicate name", []*PageAction{action(page, "save"), action(page, "save")}, "duplicate action"},
		{"default then named", []*PageAction{action(page, "default"), action(page, "save")}, "default action cannot be used with named actions"},
		{"named then default", []*PageAction{action(page, "save"), action(page, "default")}, "default action cannot be used with named actions"},
		{"layout placement", []*PageAction{action("src/routes/+layout.server.ts", "save")}, "belongs in +page.server.ts"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewActions(tc.actions...)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewActions error = %v; want %q", err, tc.want)
			}
		})
	}
}
