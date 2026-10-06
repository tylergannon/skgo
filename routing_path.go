package skgo

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Kit decode_pathname splits on literal %25, decodes URI (not component)
// escapes, then rejoins. Reserved escapes such as %2F must survive matching.
// All dynamic request paths use this before execMatchedParams decodes captures.
func decodePathname(path string) (string, error) {
	parts := strings.Split(path, "%25")
	for i, part := range parts {
		var b strings.Builder
		for j := 0; j < len(part); {
			if part[j] != '%' {
				b.WriteByte(part[j])
				j++
				continue
			}
			if j+2 >= len(part) {
				return "", fmt.Errorf("malformed pathname")
			}
			decoded, err := url.PathUnescape(part[j : j+3])
			if err != nil {
				return "", err
			}
			if strings.ContainsRune(";/?:@&=+$,#", rune(decoded[0])) {
				b.WriteString(part[j : j+3])
			} else {
				b.WriteString(decoded)
			}
			j += 3
		}
		parts[i] = b.String()
		if !utf8.ValidString(parts[i]) {
			return "", fmt.Errorf("malformed pathname UTF-8")
		}
	}
	return strings.Join(parts, "%25"), nil
}

func requestRoutingPath(u *url.URL) (string, bool) {
	path, err := decodePathname(u.EscapedPath())
	if err != nil {
		return "", false
	}
	return normalizePath(path)
}
