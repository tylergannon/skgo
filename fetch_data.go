package skgo

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
)

// fetchDataURL answers a fetch of a data: URL. Kit hands such a URL — what
// Vite makes of an asset under its inline limit — to the platform's fetch,
// which answers it from the URL itself: the Fetch standard's scheme fetch,
// with the bytes and type the data: URL processor reads out of it.
// https://fetch.spec.whatwg.org/#data-urls
func fetchDataURL(request *http.Request) (*http.Response, error) {
	u := *request.URL
	u.Fragment, u.RawFragment = "", ""
	input, ok := strings.CutPrefix(u.String(), "data:")
	if !ok {
		return nil, fmt.Errorf("skgo: fetch %q: not a data: URL", u.String())
	}
	mimeType, encoded, ok := strings.Cut(input, ",")
	if !ok {
		return nil, errors.New("skgo: fetch of a data: URL with no comma")
	}
	mimeType = strings.Trim(mimeType, "\t\n\f\r ")
	body := percentDecode(encoded)
	if i := strings.LastIndexByte(mimeType, ';'); i >= 0 && strings.EqualFold(strings.TrimLeft(mimeType[i+1:], " "), "base64") {
		decoded, err := forgivingBase64(string(body))
		if err != nil {
			return nil, fmt.Errorf("skgo: fetch of a data: URL: %w", err)
		}
		body = decoded
		mimeType = mimeType[:i]
	}
	if strings.HasPrefix(mimeType, ";") {
		mimeType = "text/plain" + mimeType
	}
	contentType, ok := serializeMIMEType(mimeType)
	if !ok {
		contentType = "text/plain;charset=US-ASCII"
	}
	return &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {contentType}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       request,
	}, nil
}

// percentDecode is the URL standard's percent-decode: a % that does not begin
// two hex digits is kept as it is.
func percentDecode(s string) []byte {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			out = append(out, unhex(s[i+1])<<4|unhex(s[i+2]))
			i += 2
			continue
		}
		out = append(out, s[i])
	}
	return out
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

func unhex(c byte) byte {
	switch {
	case c <= '9':
		return c - '0'
	case c <= 'F':
		return c - 'A' + 10
	}
	return c - 'a' + 10
}

// forgivingBase64 is the Infra standard's forgiving-base64 decode: ASCII
// whitespace is ignored, padding is optional, and leftover bits are dropped.
// https://infra.spec.whatwg.org/#forgiving-base64-decode
func forgivingBase64(s string) ([]byte, error) {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune("\t\n\f\r ", r) {
			return -1
		}
		return r
	}, s)
	if len(s)%4 == 0 {
		s = strings.TrimSuffix(s, "=")
		s = strings.TrimSuffix(s, "=")
	}
	if len(s)%4 == 1 || strings.ContainsFunc(s, func(r rune) bool {
		return !(r == '+' || r == '/' || r < 0x80 && isAlphanumeric(byte(r)))
	}) {
		return nil, errors.New("the base64 body is not valid")
	}
	return base64.RawStdEncoding.DecodeString(s)
}

func isAlphanumeric(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// serializeMIMEType parses a MIME type as the MIME Sniffing standard does and
// serializes the record, or reports that it does not parse: type and subtype
// and parameter names lowercased, the first of a repeated parameter kept, and
// a value quoted only when it has to be.
// https://mimesniff.spec.whatwg.org/#parse-a-mime-type
func serializeMIMEType(input string) (string, bool) {
	const whitespace = "\t\n\r "
	s := strings.Trim(input, whitespace)
	typ, rest, ok := strings.Cut(s, "/")
	if !ok || !isToken(typ) {
		return "", false
	}
	subtype, rest, _ := strings.Cut(rest, ";")
	subtype = strings.TrimRight(subtype, whitespace)
	if !isToken(subtype) {
		return "", false
	}
	var out strings.Builder
	out.WriteString(strings.ToLower(typ) + "/" + strings.ToLower(subtype))
	seen := map[string]bool{}
	for rest != "" {
		rest = strings.TrimLeft(rest, whitespace)
		end := strings.IndexAny(rest, ";=")
		if end < 0 {
			break
		}
		name := strings.ToLower(rest[:end])
		separator := rest[end]
		rest = rest[end+1:]
		if separator == ';' {
			continue
		}
		var value string
		if strings.HasPrefix(rest, `"`) {
			value, rest = quotedString(rest)
			_, rest, _ = strings.Cut(rest, ";")
		} else {
			value, rest, _ = strings.Cut(rest, ";")
			value = strings.TrimRight(value, whitespace)
			if value == "" {
				continue
			}
		}
		if name == "" || !isToken(name) || seen[name] || strings.ContainsFunc(value, func(r rune) bool {
			return !(r == '\t' || 0x20 <= r && r <= 0x7e || 0x80 <= r && r <= 0xff)
		}) {
			continue
		}
		seen[name] = true
		out.WriteString(";" + name + "=")
		if value != "" && isToken(value) {
			out.WriteString(value)
			continue
		}
		out.WriteString(`"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`)
	}
	return out.String(), true
}

// quotedString collects an HTTP quoted string from the opening quote s begins
// with, returning its value and what follows the closing quote.
// https://fetch.spec.whatwg.org/#collect-an-http-quoted-string
func quotedString(s string) (string, string) {
	var value strings.Builder
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '"':
			return value.String(), s[i+1:]
		case '\\':
			if i+1 == len(s) {
				value.WriteByte('\\')
				return value.String(), ""
			}
			i++
		}
		value.WriteByte(s[i])
	}
	return value.String(), ""
}

// isToken reports whether s is a non-empty run of HTTP token code points.
func isToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(isAlphanumeric(c) || strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0) {
			return false
		}
	}
	return true
}

// serverAssets is Kit's read of the files its server code imports, such as an
// asset a universal load imports by URL. Kit's `event.fetch` answers a fetch
// of one by reading the file rather than requesting it
// (`runtime/server/fetch.js`): the response carries the file's bytes, its
// length and its type, nothing of the page's credentials goes with it, and
// none of a served file's caching headers come back.
type serverAssets struct {
	// files is the build's client tree, where Kit's build copies every
	// server asset.
	files fs.FS
	// prefix is Kit's `paths.assets`, or its base when that is unset.
	prefix string
	sizes  map[string]int64
	types  map[string]string
}

func newServerAssets(build fs.FS, base string, info ManifestSSR) (*serverAssets, error) {
	if len(info.ServerAssets) == 0 {
		return nil, nil
	}
	files, err := fs.Sub(build, "client")
	if err != nil {
		return nil, err
	}
	prefix := info.Assets
	if prefix == "" {
		prefix = base
	}
	return &serverAssets{files: files, prefix: prefix, sizes: info.ServerAssets, types: info.MimeTypes}, nil
}

// read answers a fetch of decoded, a percent-decoded same-origin pathname,
// when it names a server asset, or the directory whose index.html is one.
func (a *serverAssets) read(request *http.Request, decoded string) (*http.Response, bool, error) {
	if a == nil {
		return nil, false, nil
	}
	filename := strings.TrimPrefix(decoded, a.prefix)
	if filename == "" {
		return nil, false, nil
	}
	filename = filename[1:]
	file := filename
	size, ok := a.sizes[file]
	if !ok {
		file = filename + "/index.html"
		if size, ok = a.sizes[file]; !ok {
			return nil, false, nil
		}
	}
	body, err := fs.ReadFile(a.files, file)
	if err != nil {
		return nil, true, fmt.Errorf("skgo: reading the server asset %s: %w", file, err)
	}
	var contentType string
	if i := strings.LastIndexByte(file, '.'); i >= 0 {
		contentType = a.types[file[i:]]
	}
	header := http.Header{}
	header.Set("Content-Length", strconv.FormatInt(size, 10))
	header.Set("Content-Type", contentType)
	return &http.Response{
		// A Response kit constructs has no status text.
		Status:        "200",
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: size,
		Request:       request,
	}, true, nil
}
