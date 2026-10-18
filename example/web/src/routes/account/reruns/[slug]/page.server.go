package slug

import (
	"sync"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic/visitor"
)

type PageData struct {
	Slug   string `json:"slug"`
	Filter string `json:"filter"`
	Serial int    `json:"serial"`
}

var runs = struct {
	sync.Mutex
	byVisitor map[string]int
}{byVisitor: map[string]int{}}

func pageLoad(event PageRequestEvent) (PageData, error) {
	ctx := event.Context()
	slug := event.Params.Slug()
	filter, _ := event.SearchParam("x")
	event.Depends("app:reruns")
	who := visitor.Of(ctx)
	runs.Lock()
	runs.byVisitor[who]++
	serial := runs.byVisitor[who]
	runs.Unlock()
	return PageData{Slug: slug, Filter: filter, Serial: serial}, nil
}

var _ = skgo.Load(pageLoad)
