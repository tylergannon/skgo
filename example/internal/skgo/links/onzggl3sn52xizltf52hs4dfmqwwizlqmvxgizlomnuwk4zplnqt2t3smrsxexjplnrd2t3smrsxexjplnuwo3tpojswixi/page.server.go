package typeddependencies

import (
	"crypto/rand"
	"sync"

	"github.com/tylergannon/skgo"
)

type PageData struct {
	Label   string `json:"label"`
	Ignored string `json:"ignored"`
	Serial  int    `json:"serial"`
}

var runs = struct {
	sync.Mutex
	byVisitor map[string]int
}{byVisitor: map[string]int{}}

const serialCookie = "skgo_typed_dependency_visitor"

func load(event PageRequestEvent) (PageData, error) {
	read, _ := event.SearchParam("read")
	label := "No parameter read"
	switch read {
	case "none":
	case "b":
		label = event.Params.B().Label()
	case "nested":
		event.Untrack(func() {
			event.Untrack(func() { label = event.Params.B().Label() })
			label = event.Params.A().Label()
		})
	default:
		label = event.Params.A().Label()
	}
	var ignored string
	event.Untrack(func() { ignored = event.Params.Ignored() })
	// Loads run concurrently. Assign this fixture's visitor here rather than
	// relying on the root layout to have already assigned the app's visitor.
	// Cookieless probes get their own count too; they cannot contaminate a
	// browser's first literal serial.
	who, _ := event.Cookie(serialCookie)
	if who == "" {
		who = rand.Text()
		if err := event.SetCookie(serialCookie, who, skgo.CookieOptions{Path: "/"}); err != nil {
			return PageData{}, err
		}
	}
	runs.Lock()
	runs.byVisitor[who]++
	serial := runs.byVisitor[who]
	runs.Unlock()
	return PageData{Label: label, Ignored: ignored, Serial: serial}, nil
}

var _ = skgo.Load(load)
