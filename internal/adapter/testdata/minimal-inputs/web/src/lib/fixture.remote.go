package lib

import (
	"context"
	"fmt"
	"os"

	"github.com/tylergannon/polytype/devalue"
	"github.com/tylergannon/skgo"
)

func note(line string) {
	path := os.Getenv("SKGO_INPUTS_RECEIPT")
	if path == "" {
		return
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	if _, err := fmt.Fprintln(file, line); err != nil {
		panic(err)
	}
}
func emptyInputs() ([]devalue.UndefinedValue, error) {
	note("inputs:empty")
	return []devalue.UndefinedValue{}, nil
}
func empty(context.Context) (string, error) { note("body:empty"); return "build:empty", nil }

var _ = skgo.Prerender(empty, skgo.PrerenderOptions{Inputs: emptyInputs})
