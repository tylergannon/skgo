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

// SKGO corrects the dependency's origin accessor to include non-default ports.
func TestTheEngineOriginIsTheStandardsOrigin(t *testing.T) {
	for _, c := range []struct{ expression, want string }{
		{`new URL('http://127.0.0.1:8080/a').origin`, "http://127.0.0.1:8080"},
		{`new URL('https://example.com/a?b#c').origin`, "https://example.com"},
		{`new URL('http://example.com:80/a').origin`, "http://example.com"},
		{`new URL('https://example.com:443/a').origin`, "https://example.com"},
		{`new URL('https://example.com:8443/a').origin`, "https://example.com:8443"},
		{`new URL('/x', 'http://127.0.0.1:8080/a').origin`, "http://127.0.0.1:8080"},
		{`new URL('a://thing/x').origin`, "null"},
		{`new URL('http://127.0.0.1:8080/a') instanceof URL`, "true"},
	} {
		if got := evaluate(t, c.expression); got != c.want {
			t.Errorf("%s = %q, want %q", c.expression, got, c.want)
		}
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
