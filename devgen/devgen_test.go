package devgen

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSerializedCallbackPreservesPartialChangeAndContinues(t *testing.T) {
	var out bytes.Buffer
	calls := 0
	err := ServeIO(context.Background(), strings.NewReader("{\"id\":1,\"epoch\":17,\"revision\":3,\"full\":true,\"paths\":[\"/gone/input.json\"]}\n{\"id\":2}\n"), &out, func(_ context.Context, r Request) (bool, error) {
		calls++
		if calls == 1 {
			if r.Epoch != 17 || r.Revision != 3 || !r.Full || len(r.Paths) != 1 || r.Paths[0] != "/gone/input.json" {
				t.Fatalf("selection=%+v", r)
			}
			return true, errors.New("literal partial publication")
		}
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || out.String() != "{\"id\":1,\"generated\":true,\"error\":\"literal partial publication\"}\n{\"id\":2,\"generated\":false}\n" {
		t.Fatalf("calls=%d replies=%s", calls, &out)
	}
}
func TestRejectNonMonotonicAndCorruptRequests(t *testing.T) {
	for _, input := range []string{"{\"id\":0}", "{\"id\":2}\n{\"id\":1}", "diagnostic on stdout"} {
		if err := ServeIO(context.Background(), strings.NewReader(input), &bytes.Buffer{}, func(context.Context, Request) (bool, error) { return false, nil }); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}
