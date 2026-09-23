package optional

import (
	"context"
	"fmt"
	"sync"

	"github.com/tylergannon/polytype"
	"github.com/tylergannon/skgo"
)

type Input struct {
	Name    string                    `json:"name"`
	Count   polytype.Optional[int]    `json:"count,omitzero"`
	Enabled polytype.Optional[bool]   `json:"enabled,omitzero"`
	Label   polytype.Optional[string] `json:"label,omitzero"`
}

type Result struct {
	Name       string `json:"name"`
	Count      string `json:"count"`
	Enabled    string `json:"enabled"`
	Label      string `json:"label"`
	Operations int    `json:"operations"`
}

var operations = struct {
	sync.Mutex
	byName map[string]int
}{byName: make(map[string]int)}

func submit(_ context.Context, in Input) (Result, error) {
	if in.Name == "" {
		return Result{}, skgo.Invalidf("name", "A name is required")
	}
	if in.Count.Present && in.Count.Value < 0 {
		return Result{}, skgo.Invalidf("count", "Count must be zero or greater")
	}
	result := Result{Name: in.Name, Count: "absent", Enabled: "absent", Label: "absent"}
	if in.Count.Present {
		result.Count = fmt.Sprintf("present: %d", in.Count.Value)
	}
	if in.Enabled.Present {
		result.Enabled = fmt.Sprintf("present: %t", in.Enabled.Value)
	}
	if in.Label.Present {
		result.Label = fmt.Sprintf("present: %q", in.Label.Value)
	}
	operations.Lock()
	operations.byName[in.Name]++
	result.Operations = operations.byName[in.Name]
	operations.Unlock()
	return result, nil
}

var _ = skgo.Form(submit)
