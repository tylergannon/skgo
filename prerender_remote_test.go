package skgo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPrerenderRemotePreservesNativeErrorOrigin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		call           RemoteFunc
		kind           string
		status         int
		message        string
		diagnostic     string
		wantDiagnostic bool
	}{
		{
			name: "explicit HTTP error is an app error",
			call: func(context.Context, Call) (any, error) { return nil, Errorf(404, "missing item") },
			kind: "app", status: 404, message: "missing item",
		},
		{
			name: "ordinary error remains unknown",
			call: func(context.Context, Call) (any, error) { return nil, errors.New("private detail") },
			kind: "unknown", status: 500, message: "Internal Error", diagnostic: "private detail", wantDiagnostic: true,
		},
		{
			name: "recovered panic remains unknown",
			call: func(context.Context, Call) (any, error) { panic("private panic") },
			kind: "unknown", status: 500, message: "Internal Error", diagnostic: "private panic", wantDiagnostic: true,
		},
		{
			name: "empty ordinary error retains an empty diagnostic",
			call: func(context.Context, Call) (any, error) { return nil, errors.New("") },
			kind: "unknown", status: 500, message: "Internal Error", diagnostic: "", wantDiagnostic: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fn := NewRemote(RemoteSpec{
				Kind: KindPrerender, Module: "src/data.remote.ts", Name: "item",
				Fn: func() {}, Call: test.call,
			})
			request := `{"kind":"remote","module":"src/data.remote.ts","name":"item","url":"http://skgo.test/item"}`
			var output strings.Builder
			if err := RunPrerenderBuild(strings.NewReader(request), &output, nil, nil, []*Remote{fn}); err != nil {
				t.Fatalf("RunPrerenderBuild: %v", err)
			}
			var got struct {
				Type       string    `json:"type"`
				Kind       string    `json:"kind"`
				Error      HTTPError `json:"error"`
				Diagnostic *string   `json:"diagnostic"`
			}
			if err := json.Unmarshal([]byte(output.String()), &got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			gotDiagnostic := ""
			if got.Diagnostic != nil {
				gotDiagnostic = *got.Diagnostic
			}
			if got.Type != "error" || got.Kind != test.kind || got.Error.Status != test.status || got.Error.Message != test.message || gotDiagnostic != test.diagnostic || (got.Diagnostic != nil) != test.wantDiagnostic {
				t.Fatalf("response = %#v, want error kind=%q status=%d message=%q diagnostic=%q", got, test.kind, test.status, test.message, test.diagnostic)
			}
		})
	}
}

func TestPrerenderRemoteRedirectStaysStructuredForKit(t *testing.T) {
	t.Parallel()
	fn := NewRemote(RemoteSpec{
		Kind: KindPrerender, Module: "src/data.remote.ts", Name: "item",
		Fn: func() {}, Call: func(context.Context, Call) (any, error) {
			return nil, &Redirect{Status: 308, Location: "/catalog/atlas"}
		},
	})
	request := `{"kind":"remote","module":"src/data.remote.ts","name":"item","url":"http://skgo.test/item"}`
	var output strings.Builder
	if err := RunPrerenderBuild(strings.NewReader(request), &output, nil, nil, []*Remote{fn}); err != nil {
		t.Fatalf("RunPrerenderBuild: %v", err)
	}
	var got struct {
		Type     string `json:"type"`
		Redirect struct {
			Status   int    `json:"status"`
			Location string `json:"location"`
		} `json:"redirect"`
	}
	if err := json.Unmarshal([]byte(output.String()), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Type != "redirect" || got.Redirect.Status != 308 || got.Redirect.Location != "/catalog/atlas" {
		t.Fatalf("response = %#v, want native redirect 308 /catalog/atlas", got)
	}
}
