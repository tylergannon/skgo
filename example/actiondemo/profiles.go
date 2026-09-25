package actiondemo

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"sync"

	"github.com/tylergannon/skgo"
)

type Profile struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Biography string `json:"biography"`
	State     string `json:"state"`
}

type Pair struct {
	Ada   Profile `json:"ada"`
	Grace Profile `json:"grace"`
}

var visitors = struct {
	sync.Mutex
	profiles map[string]Pair
	defaults map[string]Profile
	cross    map[string]Profile
}{profiles: map[string]Pair{}, defaults: map[string]Profile{}, cross: map[string]Profile{}}

func Visitor(ctx context.Context) (string, error) {
	event := skgo.EventFrom(ctx)
	if id, ok := event.Cookie("skgo_actions_workspace"); ok && id != "" {
		return id, nil
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := base64.RawURLEncoding.EncodeToString(raw)
	if err := event.SetCookie("skgo_actions_workspace", id, skgo.CookieOptions{Path: "/"}); err != nil {
		return "", err
	}
	return id, nil
}

func Profiles(id string) Pair {
	visitors.Lock()
	defer visitors.Unlock()
	if pair, ok := visitors.profiles[id]; ok {
		return pair
	}
	pair := Pair{
		Ada:   Profile{Name: "Ada Lovelace", Email: "ada@example.test", Biography: "First programmer", State: "Active"},
		Grace: Profile{Name: "Grace Hopper", Email: "grace@example.test", Biography: "Compiler pioneer", State: "Active"},
	}
	visitors.profiles[id] = pair
	return pair
}

func SaveSelected(id, selected string, profile Profile) bool {
	visitors.Lock()
	defer visitors.Unlock()
	pair, ok := visitors.profiles[id]
	if !ok {
		pair = Pair{
			Ada:   Profile{Name: "Ada Lovelace", Email: "ada@example.test", Biography: "First programmer", State: "Active"},
			Grace: Profile{Name: "Grace Hopper", Email: "grace@example.test", Biography: "Compiler pioneer", State: "Active"},
		}
	}
	switch selected {
	case "ada":
		pair.Ada = profile
	case "grace":
		pair.Grace = profile
	default:
		return false
	}
	visitors.profiles[id] = pair
	return true
}

func SaveDefault(id string, profile Profile) Profile {
	visitors.Lock()
	defer visitors.Unlock()
	visitors.defaults[id] = profile
	return visitors.defaults[id]
}

func DefaultProfile(id string) (Profile, bool) {
	visitors.Lock()
	defer visitors.Unlock()
	profile, ok := visitors.defaults[id]
	return profile, ok
}

func CrossProfile(id string) Profile {
	visitors.Lock()
	defer visitors.Unlock()
	if profile, ok := visitors.cross[id]; ok {
		return profile
	}
	return Profile{Name: "Ada Lovelace", Email: "ada@example.test", Biography: "First programmer", State: "Active"}
}

func SaveCrossProfile(id string, profile Profile) {
	visitors.Lock()
	defer visitors.Unlock()
	visitors.cross[id] = profile
}
