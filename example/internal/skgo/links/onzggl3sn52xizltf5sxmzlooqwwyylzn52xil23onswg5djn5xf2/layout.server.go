package eventlayout

import (
	"crypto/rand"
	"sync"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/internal/app"
)

type LayoutData struct {
	Section string `json:"eventSection"`
	Item    string `json:"eventItem"`
	Route   string `json:"eventRoute"`
	User    string `json:"eventUser"`
	Serial  int    `json:"eventSerial"`
}

var runs = struct {
	sync.Mutex
	byVisitor map[string]int
}{byVisitor: map[string]int{}}

func layoutLoad(event LayoutRequestEvent) (LayoutData, error) {
	data := LayoutData{Section: event.Params.Section(), User: event.Locals.Session.User}
	if app.LocalsFrom(event.Context()) != event.Locals {
		return data, skgo.Errorf(500, "layout event and context locals differ")
	}
	read := func() {
		// Item belongs only to descendant pages, so it is optional here.
		if item := event.Params.Item(); item != nil {
			data.Item = *item
		}
		data.Route = event.RouteID()
	}
	mode, _ := event.SearchParam("route")
	if mode == "tracked" {
		read()
	} else {
		event.Untrack(read)
	}
	who, _ := event.Cookie("skgo_event_layout_visitor")
	if who == "" {
		who = rand.Text()
		if err := event.SetCookie("skgo_event_layout_visitor", who, skgo.CookieOptions{Path: "/"}); err != nil {
			return data, err
		}
	}
	runs.Lock()
	runs.byVisitor[who]++
	data.Serial = runs.byVisitor[who]
	runs.Unlock()
	return data, nil
}

var _ = skgo.Load(layoutLoad)
