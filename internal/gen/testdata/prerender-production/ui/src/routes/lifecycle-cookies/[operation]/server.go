package lifecyclecookies

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/tylergannon/skgo"
)

func get(w http.ResponseWriter, r *http.Request) {
	operation := path.Base(r.URL.Path)
	if i := strings.LastIndex(operation, "-"); i >= 0 {
		operation = operation[i+1:]
	}
	switch operation {
	case "set":
		for _, cookie := range []*http.Cookie{
			{Name: "flavor", Value: "root", Path: "/", HttpOnly: true},
			{Name: "flavor", Value: "scoped%20raw", Path: "/lifecycle", Domain: "127.0.0.1", SameSite: http.SameSiteStrictMode},
			{Name: "hidden", Value: "wrong-path", Path: "/outside"},
			{Name: "foreign", Value: "wrong-domain", Path: "/", Domain: "other.test"},
		} {
			http.SetCookie(w, cookie)
		}
	case "delete":
		http.SetCookie(w, &http.Cookie{Name: "erase", Value: "", Path: "/", MaxAge: -1})
	case "verify":
		flavor, _ := r.Cookie("flavor")
		erase, _ := r.Cookie("erase")
		_, hidden := r.Cookie("hidden")
		_, foreign := r.Cookie("foreign")
		if flavor == nil || flavor.Value != "root" || erase == nil || erase.Value != "" || hidden == nil || foreign == nil {
			http.Error(w, "cookie attributes lost: "+r.Header.Get("Cookie"), 500)
			return
		}
		fmt.Fprint(w, "cookie-forwarding:root")
		return
	default:
		http.NotFound(w, r)
		return
	}
	fmt.Fprint(w, "cookies updated")
}

var _ = skgo.GET(get)
