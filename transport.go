package skgo

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/tylergannon/skgo/internal/devalue"
	"github.com/tylergannon/skgo/internal/remotearg"
)

// Transporter is one member of kit's `transport` hook — the server half of the
// `encode`/`decode` pair the app declares in `src/hooks.ts`.
//
// Kit's own shape is
//
//	export interface Transporter<T, U> {
//		encode: (value: T) => false | U;
//		decode: (data: U) => T;
//	}
//
// and the two halves sit on opposite sides of the wire. The browser runs kit's
// client, so `decode` is the app's TypeScript: it is what turns the encoding
// back into an instance of a class, which is the whole point — a domain type
// that arrives as a plain object has lost its methods. Go runs the server, so
// this is the half that decides whether a Go value is the custom type and what
// the browser receives for it.
//
// Kit's `encode` answers two questions at once — is this my type, and what is
// its encoding — because JavaScript spells the first as `value instanceof
// Money`. Go spells it as a type, so Type asks it and Encode answers only the
// second.
//
// Type is not decoration. skgo has to know, before it serializes anything,
// which Go types must survive the trip to devalue intact; see Transport.
type Transporter struct {
	// Type is the Go type this transports — kit's `instanceof`.
	Type reflect.Type
	// Encode returns what the browser's `decode` receives: a devalue value
	// model tree, the same shapes devalue.Stringify accepts. v is of Type.
	Encode func(v any) (encoded any, err error)
	// Decode turns what the browser's `encode` produced back into a Go value
	// of Type.
	Decode func(encoded any) (any, error)
}

// Transport is kit's `transport` hook: custom types keyed by name.
//
// The keys are the wire, not a Go detail. Kit's client builds its decoders from
// the same object skgo is mirroring here —
//
//	export const transport = {
//		Money: { encode: (v) => v instanceof Money && [v.cents], decode: ([cents]) => new Money(cents) }
//	};
//
// — so a key skgo serializes under and a key `src/hooks.ts` declares must be
// spelled identically. A value goes out as `["Money", <slot>]`, and a client
// with no `Money` decoder fails to parse the response rather than quietly
// receiving a plain object.
//
// Kit puts the transport on a *universal* hook because both sides need it, and
// skgo keeps that: the browser half stays in `src/hooks.ts` where kit looks for
// it, and this is the server half.
//
// # Why a Transport changes how results are encoded
//
// Kit hands devalue the real object graph, so its encoders see a `Money`
// instance. skgo cannot: devalue refuses an arbitrary Go struct, so a result
// reaches it through a `json.Marshal`/`json.Unmarshal` round trip that has
// already turned every struct into a `map[string]any`. A reducer running after
// that sees an anonymous map and has nothing left to recognise — which is
// precisely why a domain type arrives in the browser as a plain object today.
//
// So a non-empty Transport switches the outbound encoding to a walk that
// descends into the Go value along any path that can reach a transported type
// and leaves the transported values themselves in place, where devalue's
// reducers can still see them. Everything the walk cannot reach still goes
// through encoding/json exactly as before.
type Transport map[string]Transporter

// validate refuses a transport that could not round-trip, at construction
// rather than on the first request that happens to carry the type.
func (t Transport) validate() error {
	for _, key := range t.keys() {
		if key == "" {
			return fmt.Errorf("skgo: a transport key is empty; it has to be the name src/hooks.ts declares")
		}
		// kit spells the key straight into the wire document and the client
		// looks it up by that string. A key holding a quote would produce a
		// document its own client cannot read.
		if strings.ContainsAny(key, "\"\\") {
			return fmt.Errorf("skgo: transport key %q contains a quote or backslash; kit writes the key into the wire document verbatim", key)
		}
		if t[key].Type == nil {
			return fmt.Errorf("skgo: transport key %q has no Type; skgo needs the Go type to know what must reach devalue intact", key)
		}
		if t[key].Encode == nil {
			return fmt.Errorf("skgo: transport key %q has no Encode; kit's transporters are a pair", key)
		}
		if t[key].Decode == nil {
			return fmt.Errorf("skgo: transport key %q has no Decode; kit's transporters are a pair", key)
		}
	}
	return nil
}

// keys returns the transport's keys in a fixed order.
//
// Kit iterates its transport object in insertion order and devalue tries
// reducers in the order it is given them, so with two transporters that both
// claim one value the first one wins. A Go map has no order, which would make
// that choice differ between runs of the same binary; sorting makes it a
// property of the app rather than of the map's iteration. Two transporters
// claiming one value is a bug either way — this only decides that the bug
// reproduces.
func (t Transport) keys() []string {
	keys := make([]string, 0, len(t))
	for key := range t {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// reducers is the transport as devalue reducers, for everything Go serializes
// towards the browser.
func (t Transport) reducers() []devalue.Reducer {
	if len(t) == 0 {
		return nil
	}
	out := make([]devalue.Reducer, 0, len(t))
	for _, key := range t.keys() {
		transporter := t[key]
		out = append(out, devalue.Reducer{Key: key, Fn: func(v any) (any, bool, error) {
			if reflect.TypeOf(v) != transporter.Type {
				return nil, false, nil
			}
			encoded, err := transporter.Encode(v)
			if err != nil {
				return nil, false, err
			}
			return encoded, true, nil
		}})
	}
	return out
}

// unevalReplacer is the transport as a devalue `uneval` replacer, for the one
// place a value is written as JavaScript rather than as the flat wire format:
// the document skgo renders.
//
// It is kit's own replacer, from `packages/kit/src/runtime/app/internal/
// transport.js`:
//
//	const replacer = (thing) => {
//		for (const key of Object.keys(transport)) {
//			const encoded = transport[key].encode(thing);
//			if (encoded) {
//				return `app.decode('${key}', ${devalue.uneval(encoded, replacer)})`;
//			}
//		}
//	};
//
// `app` is the client's app module, which the boot script has already imported
// by the time either the hydration array or `<global>.data` is evaluated, and
// `app.decode(type, value)` is the client half of the same hook — so a value
// written this way arrives in the browser as an instance of the app's class
// rather than as the object its fields were carried in.
func (t Transport) unevalReplacer() devalue.Replacer {
	if len(t) == 0 {
		return nil
	}
	keys := t.keys()
	return func(v any, uneval func(any) (string, error)) (string, bool, error) {
		for _, key := range keys {
			transporter := t[key]
			if reflect.TypeOf(v) != transporter.Type {
				continue
			}
			encoded, err := transporter.Encode(v)
			if err != nil {
				return "", false, err
			}
			written, err := uneval(encoded)
			if err != nil {
				return "", false, err
			}
			return "app.decode(" + devalue.QuoteString(key) + ", " + written + ")", true, nil
		}
		return "", false, nil
	}
}

// revivers is the transport as devalue revivers, for everything Go parses that
// the browser serialized.
func (t Transport) revivers() map[string]func(any) (any, error) {
	if len(t) == 0 {
		return nil
	}
	out := make(map[string]func(any) (any, error), len(t))
	for key, transporter := range t {
		out[key] = transporter.Decode
	}
	return out
}

// argumentRevivers is the transport as devalue revivers for a remote
// function's *argument*, where the reviver resolves the tag and hands the
// payload on unchanged.
//
// Kit's client tags a transported argument as `["Money", <payload>]`, and kit's
// server revives it before its schema validates the result. skgo's schema is
// the decoder polytype generated from the Go parameter type — and for a
// transported type that decoder is the very function the `transport` hook's
// Decode calls, because `skgo generate` emits both from one lowering. Reviving
// here would run that decoder now and leave the generated decoder looking at a
// Go value instead of the shape it validates; so the tag is resolved, the
// payload is carried through, and the generated decoder does the one decode
// there is. An unknown key still fails to parse, exactly as it does in kit.
func (t Transport) argumentRevivers() map[string]func(any) (any, error) {
	if len(t) == 0 {
		return nil
	}
	out := make(map[string]func(any) (any, error), len(t))
	for key := range t {
		out[key] = func(v any) (any, error) { return v, nil }
	}
	return out
}

// codecs is the registry's transport in the form internal/remotearg wants for
// an incoming argument.
func (rs *Remotes) codecs() remotearg.Codecs {
	return remotearg.Codecs{
		Reducers: rs.cfg.Transport.reducers(),
		Revivers: rs.cfg.Transport.argumentRevivers(),
	}
}

// parsePayload turns one payload into the call a generated closure is handed.
func (rs *Remotes) parsePayload(payload string) (Call, error) {
	arg, present, err := remotearg.ParsePayloadWith(payload, rs.codecs())
	if err != nil {
		return Call{}, err
	}
	return rs.newCall(arg, present), nil
}

// Transported declares that T crosses the wire as a custom type under key, and
// is skgo's mirror of one entry of kit's `transport` hook.
//
// It goes in `src/hooks.go`, beside the `src/hooks.ts` that holds the browser
// half, because kit's transport is a universal hook and both halves are one
// declaration:
//
//	// src/hooks.go
//	var _ = skgo.Transported[businesslogic.Money]("Money")
//
//	// src/hooks.ts
//	export const transport = {
//		Money: {
//			encode: (v) => v instanceof Money && [v.cents],
//			decode: ([cents]) => new Money(cents)
//		}
//	};
//
// `skgo generate` reads the marker, projects T's shape with polytype, and emits
// the Go encoder and strict decoder that answer the browser's pair. Like every
// other skgo marker it is a declaration and not a call: it does nothing at run
// time.
func Transported[T any](key string) Marker { _ = key; return Marker{} }
