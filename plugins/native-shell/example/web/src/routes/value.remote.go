package value

import (
	"context"
	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/plugins/native-shell/example/internal/skgo/params"
	"sync"
)

var mu sync.Mutex
var value = "initial"

func readValue(ctx context.Context) (string, error) { mu.Lock(); defer mu.Unlock(); return value, nil }
func writeValue(ctx context.Context, _ params.RequestEvent, next string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	value = next
	return value, nil
}

var _ = skgo.Query(readValue)
var _ = skgo.Command(writeValue)
