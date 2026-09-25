package skgo

import (
	"net/http"
	"net/url"
	"strings"
)

// rejectReservedQuery mirrors Kit's check in runtime/server/respond.js. Kit
// removes its two data-request parameters before checking the remaining URL;
// a remote call with a pathname header uses the companion search header as its
// event URL instead of the remote endpoint's query.
func rejectReservedQuery(w http.ResponseWriter, r *http.Request, data, remote bool) bool {
	raw := r.URL.RawQuery
	if remote {
		if _, hasPathname := r.Header[http.CanonicalHeaderKey("x-sveltekit-pathname")]; hasPathname {
			raw = strings.TrimPrefix(r.Header.Get("x-sveltekit-search"), "?")
		}
	}
	for _, pair := range strings.Split(raw, "&") {
		key, _, _ := strings.Cut(pair, "=")
		name := queryKey(key)
		if data && (name == trailingSlashParam || name == invalidatedParam) {
			continue
		}
		if strings.HasPrefix(name, "x-sveltekit-") {
			w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Cannot use reserved query parameter \"" + name + "\""))
			return true
		}
	}
	return false
}

// URLSearchParams keeps malformed percent escapes as literal text while still
// decoding the valid escapes around them. QueryUnescape rejects the whole key,
// so protect malformed percent signs before decoding it.
func queryKey(raw string) string {
	if name, err := url.QueryUnescape(raw); err == nil {
		return name
	}
	var escaped strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] == '%' && (i+2 >= len(raw) || !isHexDigit(raw[i+1]) || !isHexDigit(raw[i+2])) {
			escaped.WriteString("%25")
		} else {
			escaped.WriteByte(raw[i])
		}
	}
	name, _ := url.QueryUnescape(escaped.String())
	return name
}

func isHexDigit(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}
