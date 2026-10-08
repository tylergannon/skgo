package skgo

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tylergannon/devalue/v5"
)

func TestPrerenderServiceReturnsDeclaredInputsWithoutRequestContext(t *testing.T) {
	called := false
	fn := NewRemote(RemoteSpec{
		Kind:   KindPrerender,
		Module: "src/routes/catalog/catalog.remote.ts",
		Name:   "item",
		Call:   func(context.Context, Call) (any, error) { return nil, nil },
		Inputs: func(ctx context.Context, call Call) ([]any, error) {
			called = true
			if ctx == nil || EventFrom(ctx) != nil {
				t.Fatalf("input context = %#v, want a plain build context without Event", ctx)
			}
			return []any{"atlas", "beacon"}, nil
		},
	})
	request := `{"module":"src/routes/catalog/catalog.remote.ts","name":"item"}`
	var output strings.Builder
	if err := callBuildOperation("/inputs", strings.NewReader(request), &output, nil, nil, []*Remote{fn}); err != nil {
		t.Fatalf("prerender service: %v", err)
	}
	if !called {
		t.Fatal("input producer was not called")
	}
	var response struct {
		Inputs string `json:"inputs"`
	}
	if err := json.Unmarshal([]byte(output.String()), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	got, err := devalue.Parse(response.Inputs, nil)
	if err != nil {
		t.Fatalf("parse devalue inputs: %v", err)
	}
	if want := []any{"atlas", "beacon"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded inputs = %#v, want %#v", got, want)
	}
}

func TestPrerenderServiceInputsErrorsFailBuildAndRejectExtraFields(t *testing.T) {
	producerErr := errors.New("inputs fixture failed")
	fn := NewRemote(RemoteSpec{
		Kind: KindPrerender, Module: "mod.remote.ts", Name: "data",
		Call:   func(context.Context, Call) (any, error) { return nil, nil },
		Inputs: func(context.Context, Call) ([]any, error) { return nil, producerErr },
	})
	request := `{"module":"mod.remote.ts","name":"data"}`
	var output strings.Builder
	err := callBuildOperation("/inputs", strings.NewReader(request), &output, nil, nil, []*Remote{fn})
	if err == nil || !strings.Contains(err.Error(), "inputs fixture failed") {
		t.Fatalf("producer error = %v; want propagated failure", err)
	}
	extra := `{"module":"mod.remote.ts","name":"data","url":"http://unused"}`
	output.Reset()
	if err := callBuildOperation("/inputs", strings.NewReader(extra), &output, nil, nil, []*Remote{fn}); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("extra-field request error = %v; want strict protocol rejection", err)
	}
}

func TestPrerenderServiceInputsSerializesExplicitUndefined(t *testing.T) {
	fn := NewRemote(RemoteSpec{
		Kind: KindPrerender, Module: "mod.remote.ts", Name: "empty",
		Call:   func(context.Context, Call) (any, error) { return nil, nil },
		Inputs: func(context.Context, Call) ([]any, error) { return []any{devalue.UndefinedValue{}}, nil },
	})
	var output strings.Builder
	request := `{"module":"mod.remote.ts","name":"empty"}`
	if err := callBuildOperation("/inputs", strings.NewReader(request), &output, nil, nil, []*Remote{fn}); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Inputs string `json:"inputs"`
	}
	if err := json.Unmarshal([]byte(output.String()), &response); err != nil {
		t.Fatal(err)
	}
	got, err := devalue.Parse(response.Inputs, nil)
	if err != nil {
		t.Fatal(err)
	}
	values, ok := got.([]any)
	if !ok || len(values) != 1 {
		t.Fatalf("decoded inputs = %#v, want one explicit undefined", got)
	}
	if _, ok := values[0].(devalue.UndefinedValue); !ok {
		t.Fatalf("input = %#v, want devalue.UndefinedValue", values[0])
	}
}
