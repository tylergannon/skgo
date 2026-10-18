package app

import "strings"

// SerializedHeader chooses which response headers a universal load's
// hydration data carries. The handle hook and the prerender service pass the
// same filter, so a prerendered page and a live one serialize the same headers.
func SerializedHeader(name, value string) bool {
	switch name {
	case "x-fetch-hook", "x-replay-allowed":
		return true
	case "set-cookie":
		return strings.HasPrefix(value, "lamp=")
	}
	return false
}
