package ssr

import (
	"encoding/base64"
	"fmt"
	"unicode/utf8"

	"github.com/dop251/goja"
	gojaurl "github.com/dop251/goja_nodejs/url"
)

// installWebGlobals binds the web platform's data-shaping globals natively.
//
// Kit's runtime constructs a `URL` at module scope (`utils/url.js`) and a
// `TextEncoder`, a `TextDecoder`, `btoa` and `atob` (`runtime/utils.js`), so a
// bare engine cannot even evaluate the bundle without them. They used to be
// three hundred lines of JavaScript inside the adapter — a URL parser and a
// UTF-8 codec, hand-written, shipped in every bundle and reviewed by nobody —
// which is a strange thing for a project whose rule is that no application work
// happens in JavaScript. None of them does any I/O; they are pure functions of
// their arguments, and Go has all of them already.
//
// They are bound before the bundle is evaluated, which is also what makes the
// bundle's banner able to be small: the polyfill it carries is only the three
// classes kit compares against with `instanceof`, which a Go-backed constructor
// could not satisfy.
func (rt *runtime) installWebGlobals() error {
	if err := rt.installURL(); err != nil {
		return err
	}
	if err := rt.installTextCodecs(); err != nil {
		return err
	}
	return rt.installBase64()
}

// installURL binds WHATWG `URL` and `URLSearchParams` from goja_nodejs, which
// implements them against Go's own net/url.
//
// goja_nodejs publishes them as a CommonJS module, and its `Enable` helper
// reaches them through a `require` registry — which would put a `require`
// function in the engine that can read files. The module's loader is called
// directly instead, so nothing in the engine gains a way to load anything.
//
// It departs from the standard in one place: its `origin` is the scheme and the
// hostname, with the port left out, and it defines that accessor as
// non-configurable so it cannot be corrected from outside. Nothing the engine
// runs reads it — the origin comparisons in kit's runtime are in `csrf.js`,
// `fetch.js` and `load_data.js`, none of which the engine executes, because Go
// makes those decisions. webglobals_test.go pins the deviation so that a future
// render path reaching an origin finds it written down rather than discovering
// that two ports look like one host.
func (rt *runtime) installURL() error {
	module := rt.vm.NewObject()
	if err := module.Set("exports", rt.vm.NewObject()); err != nil {
		return err
	}
	gojaurl.Require(rt.vm, module)

	exports := module.Get("exports").ToObject(rt.vm)
	for _, name := range []string{"URL", "URLSearchParams"} {
		value := exports.Get(name)
		if value == nil {
			return fmt.Errorf("skgo: goja_nodejs's url module exports no %s", name)
		}
		if err := rt.vm.Set(name, value); err != nil {
			return err
		}
	}
	return nil
}

// installTextCodecs binds `TextEncoder` and `TextDecoder`. Only UTF-8 exists
// here, which is all kit asks for: `runtime/utils.js` constructs both with no
// arguments and uses them to move between a string and its bytes.
func (rt *runtime) installTextCodecs() error {
	uint8Array, ok := goja.AssertConstructor(rt.vm.Get("Uint8Array"))
	if !ok {
		return fmt.Errorf("skgo: this engine has no Uint8Array to encode text into")
	}

	encoder := func(call goja.ConstructorCall) *goja.Object {
		this := call.This
		mustSet(rt.vm, this, "encoding", rt.vm.ToValue("utf-8"))
		mustSet(rt.vm, this, "encode", rt.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			text := ""
			if arg := call.Argument(0); !goja.IsUndefined(arg) {
				text = arg.String()
			}
			bytes := rt.vm.NewArrayBuffer([]byte(text))
			array, err := uint8Array(nil, rt.vm.ToValue(bytes))
			if err != nil {
				panic(rt.vm.NewGoError(err))
			}
			return array
		}))
		return nil
	}

	decoder := func(call goja.ConstructorCall) *goja.Object {
		this := call.This
		mustSet(rt.vm, this, "encoding", rt.vm.ToValue("utf-8"))
		mustSet(rt.vm, this, "decode", rt.vm.ToValue(func(call goja.FunctionCall) goja.Value {
			arg := call.Argument(0)
			if goja.IsUndefined(arg) || goja.IsNull(arg) {
				return rt.vm.ToValue("")
			}
			var bytes []byte
			// goja exports an ArrayBuffer and anything backed by one — a typed
			// array, a DataView — straight into []byte.
			if err := rt.vm.ExportTo(arg, &bytes); err != nil {
				panic(rt.vm.NewTypeError("skgo: TextDecoder.decode wants bytes: %s", err))
			}
			if !utf8.Valid(bytes) {
				// What the real one does: an invalid sequence becomes U+FFFD
				// rather than an error, because `fatal` defaults to false.
				return rt.vm.ToValue(string([]rune(string(bytes))))
			}
			return rt.vm.ToValue(string(bytes))
		}))
		return nil
	}

	if err := rt.vm.Set("TextEncoder", encoder); err != nil {
		return err
	}
	return rt.vm.Set("TextDecoder", decoder)
}

// installBase64 binds `btoa` and `atob`. Both are defined over binary strings —
// one code unit per byte — which is exactly what kit's `base64_encode` and
// `base64_decode` hand them after a TextEncoder round trip.
func (rt *runtime) installBase64() error {
	if err := rt.vm.Set("btoa", func(call goja.FunctionCall) goja.Value {
		binary := call.Argument(0).String()
		bytes := make([]byte, 0, len(binary))
		for _, code := range []rune(binary) {
			if code > 0xff {
				panic(rt.vm.NewTypeError(
					"skgo: btoa was given a character outside the latin1 range (U+%04X)", code))
			}
			bytes = append(bytes, byte(code))
		}
		return rt.vm.ToValue(base64.StdEncoding.EncodeToString(bytes))
	}); err != nil {
		return err
	}

	return rt.vm.Set("atob", func(call goja.FunctionCall) goja.Value {
		encoded := call.Argument(0).String()
		// The web's decoder ignores padding that is absent and whitespace that
		// is present; Go's strict one refuses both.
		bytes, err := base64.RawStdEncoding.DecodeString(trimBase64(encoded))
		if err != nil {
			panic(rt.vm.NewTypeError("skgo: atob was given text that is not base64: %s", err))
		}
		runes := make([]rune, len(bytes))
		for i, b := range bytes {
			runes[i] = rune(b)
		}
		return rt.vm.ToValue(string(runes))
	})
}

// trimBase64 drops the padding and the whitespace the web's atob tolerates and
// Go's decoder does not.
func trimBase64(s string) string {
	out := make([]byte, 0, len(s))
	for i := range len(s) {
		switch c := s[i]; c {
		case '=', ' ', '\t', '\n', '\r', '\f':
		default:
			out = append(out, c)
		}
	}
	return string(out)
}

// mustSet is for properties of an object this package just created, where a
// failure would mean the engine itself is broken rather than that the app is.
func mustSet(vm *goja.Runtime, object *goja.Object, name string, value goja.Value) {
	if err := object.Set(name, value); err != nil {
		panic(vm.NewGoError(err))
	}
}
