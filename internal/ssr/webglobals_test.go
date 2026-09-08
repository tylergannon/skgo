package ssr

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// The engine's URL, text codecs and base64 are bound from Go. They used to be
// three hundred lines of JavaScript shipped inside every SSR bundle — a URL
// parser and a UTF-8 codec written by hand, which nothing checked.
//
// Every expectation below is written here, from the WHATWG URL standard and
// RFC 4648, rather than read back out of the implementation.

// evaluate runs one expression in a runtime carrying exactly the globals a real
// render gets, and returns it as a string.
func evaluate(t *testing.T, expression string) string {
	t.Helper()
	rt := &runtime{vm: goja.New()}
	if err := rt.installGlobals(func(string, string, string) {}); err != nil {
		t.Fatal(err)
	}
	if err := rt.installWebGlobals(); err != nil {
		t.Fatal(err)
	}
	value, err := rt.vm.RunString(expression)
	if err != nil {
		t.Fatalf("%s: %v", expression, err)
	}
	return value.String()
}

func TestTheEngineParsesAURLTheWayTheWebDoes(t *testing.T) {
	for _, c := range []struct {
		expression string
		want       string
	}{
		// The shape entry.js parses on every render.
		{`new URL('http://127.0.0.1:8080/items/93?from=nav#top').pathname`, "/items/93"},
		{`new URL('http://127.0.0.1:8080/items/93?from=nav#top').search`, "?from=nav"},
		{`new URL('http://127.0.0.1:8080/items/93?from=nav#top').hash`, "#top"},
		{`new URL('http://127.0.0.1:8080/a').port`, "8080"},
		{`new URL('http://127.0.0.1:8080/a').host`, "127.0.0.1:8080"},
		{`new URL('http://127.0.0.1:8080/a').protocol`, "http:"},
		// A relative reference against a base, which is how kit builds a link.
		{`new URL('../c', 'https://example.com/a/b/x').href`, "https://example.com/c"},
		{`new URL('/d', 'https://example.com/a/b').href`, "https://example.com/d"},
		// Percent-encoding survives, and a space in a query becomes %20.
		{`new URL('https://example.com/a b?q=x y').search`, "?q=x%20y"},
		// The one kit's `utils/url.js` constructs at module scope, which is why
		// a runtime with no URL cannot even evaluate the bundle.
		{`new URL('a://').protocol`, "a:"},
		// Search parameters, which kit reads a remote function's payload out of.
		{`new URL('https://e.com/?payload=abc&x=1').searchParams.get('payload')`, "abc"},
		{`new URLSearchParams('a=1&a=2').getAll('a').join(',')`, "1,2"},
		{`String(new URLSearchParams({ a: '1', b: 'two words' }))`, "a=1&b=two+words"},
	} {
		if got := evaluate(t, c.expression); got != c.want {
			t.Errorf("%s = %q, want %q", c.expression, got, c.want)
		}
	}
}

// TestTheEngineOriginLeavesThePortOut pins the one place the engine's URL is
// not the web's. The standard says the origin of `http://127.0.0.1:8080/` is
// `http://127.0.0.1:8080`; goja_nodejs answers `http://127.0.0.1`, and defines
// the accessor as non-configurable, so it cannot be corrected from outside.
//
// This is written down rather than fixed because nothing the engine runs reads
// an origin: kit compares origins in `csrf.js`, `fetch.js` and `load_data.js`,
// and skgo runs none of them — Go makes those decisions. The day a render path
// does reach one, this test fails to be a curiosity and becomes the reason two
// apps on one machine look like the same site.
func TestTheEngineOriginLeavesThePortOut(t *testing.T) {
	const standard = "http://127.0.0.1:8080"
	got := evaluate(t, `new URL('http://127.0.0.1:8080/a').origin`)
	if got == standard {
		t.Fatalf("the engine's URL now gives the standard origin %q; "+
			"delete this test and assert the standard in the table above", standard)
	}
	if got != "http://127.0.0.1" {
		t.Fatalf("the engine's origin is %q, which is neither the standard %q "+
			"nor the deviation this test was written against", got, standard)
	}
}

func TestTheEngineRefusesAURLThatIsNotOne(t *testing.T) {
	rt := &runtime{vm: goja.New()}
	if err := rt.installGlobals(func(string, string, string) {}); err != nil {
		t.Fatal(err)
	}
	if err := rt.installWebGlobals(); err != nil {
		t.Fatal(err)
	}
	// Kit's Redirect constructor builds a URL inside a try/catch to decide
	// whether a location is usable. A parser that accepted anything would let
	// an unusable location through as a 3xx nobody can follow.
	if _, err := rt.vm.RunString(`new URL('not a url')`); err == nil {
		t.Fatal("the engine accepted 'not a url' as a URL")
	}
}

func TestTheEngineEncodesTextAsUTF8(t *testing.T) {
	// "héllo ☕" is 7 characters and 10 bytes: h, 2 for é, l, l, o, space, 3
	// for ☕ — counted here rather than measured with the encoder.
	if got := evaluate(t, `new TextEncoder().encode('héllo ☕').length`); got != "10" {
		t.Errorf("the encoded length is %s, want 10", got)
	}
	if got := evaluate(t, `Array.from(new TextEncoder().encode('é')).join(',')`); got != "195,169" {
		t.Errorf("é encodes to %s, want 195,169", got)
	}
	if got := evaluate(t, `new TextEncoder().encode('abc') instanceof Uint8Array`); got != "true" {
		t.Errorf("encode returned something that is not a Uint8Array (%s)", got)
	}
	if got := evaluate(t, `new TextEncoder().encoding`); got != "utf-8" {
		t.Errorf("the encoder calls itself %q, want utf-8", got)
	}
}

func TestTheEngineDecodesUTF8BackToTheSameText(t *testing.T) {
	// The round trip kit's `runtime/utils.js` performs, and the bytes of the
	// coffee cup written out by hand.
	if got := evaluate(t, `new TextDecoder().decode(new TextEncoder().encode('héllo ☕'))`); got != "héllo ☕" {
		t.Errorf("the round trip gave %q", got)
	}
	if got := evaluate(t, `new TextDecoder().decode(new Uint8Array([226, 152, 149]))`); got != "☕" {
		t.Errorf("226,152,149 decodes to %q, want ☕", got)
	}
	if got := evaluate(t, `new TextDecoder().decode(new Uint8Array([]))`); got != "" {
		t.Errorf("no bytes decode to %q, want the empty string", got)
	}
}

func TestTheEngineMovesBetweenBinaryAndBase64(t *testing.T) {
	// From RFC 4648's own examples, which is where the padding rules are too.
	for _, c := range []struct{ text, want string }{
		{"f", "Zg=="},
		{"fo", "Zm8="},
		{"foo", "Zm9v"},
		{"foob", "Zm9vYg=="},
		{"fooba", "Zm9vYmE="},
		{"foobar", "Zm9vYmFy"},
	} {
		if got := evaluate(t, `btoa(`+quote(c.text)+`)`); got != c.want {
			t.Errorf("btoa(%q) = %q, want %q", c.text, got, c.want)
		}
		if got := evaluate(t, `atob(`+quote(c.want)+`)`); got != c.text {
			t.Errorf("atob(%q) = %q, want %q", c.want, got, c.text)
		}
	}

	// The pair kit uses to carry a byte string: encode to UTF-8, then base64.
	// "é" is two bytes, 0xC3 0xA9, which base64 writes as "w6k=".
	if got := evaluate(t, `btoa(String.fromCharCode(0xc3, 0xa9))`); got != "w6k=" {
		t.Errorf("the two bytes of é base64 to %q, want w6k=", got)
	}
	if got := evaluate(t, `atob('w6k=').split('').map((c) => c.charCodeAt(0)).join(',')`); got != "195,169" {
		t.Errorf("w6k= decodes to %s, want 195,169", got)
	}
}

func TestTheEngineRefusesToBase64ACharacterThatIsNotAByte(t *testing.T) {
	rt := &runtime{vm: goja.New()}
	if err := rt.installGlobals(func(string, string, string) {}); err != nil {
		t.Fatal(err)
	}
	if err := rt.installWebGlobals(); err != nil {
		t.Fatal(err)
	}
	// btoa is defined over binary strings. Handing it text is a mistake the
	// caller has to hear about, not one to encode a mangled answer for.
	_, err := rt.vm.RunString(`btoa('☕')`)
	if err == nil {
		t.Fatal("btoa encoded a character outside the latin1 range")
	}
	if !strings.Contains(err.Error(), "latin1") {
		t.Errorf("the refusal does not say why: %v", err)
	}
}

// TestTheEngineHasNoWayToLoadAnything is the rule the whole engine obeys: no
// application I/O executes in JavaScript. WHATWG URL comes from goja_nodejs,
// which publishes it as a CommonJS module, and the obvious way to reach it —
// its `Enable` helper — installs a `require` that can read files off disk.
//
// `setTimeout` is deliberately not in this list: the engine has one, and it is
// a queue rather than a timer — kit's `query.batch` needs a macrotask, and the
// one installed in globals.go never consults a clock. `setInterval` is, because
// there is nothing in a render that a repeating callback could be for.
func TestTheEngineHasNoWayToLoadAnything(t *testing.T) {
	for _, name := range []string{"require", "process", "fetch", "XMLHttpRequest", "WebSocket"} {
		if got := evaluate(t, `typeof `+name); got != "undefined" {
			t.Errorf("the engine has a %s (typeof is %q)", name, got)
		}
	}

	rt := &runtime{vm: goja.New()}
	if err := rt.installGlobals(func(string, string, string) {}); err != nil {
		t.Fatal(err)
	}
	if err := rt.installWebGlobals(); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.vm.RunString(`setInterval(() => {}, 1000)`); err == nil {
		t.Error("the engine let a render start a repeating timer")
	}
}

func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "\\'") + "'"
}
