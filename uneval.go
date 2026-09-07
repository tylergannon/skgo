package skgo

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/tylergannon/skgo/internal/devalue"
)

// uneval writes the JavaScript that evaluates to v, which is what a document's
// boot script carries: devalue's `uneval`, not its `stringify`.
//
// It is deliberately narrow. Everything it is given has already been through
// `encodeValue`, so it is the plain tree encoding/json produces — objects,
// arrays, strings, numbers, booleans and null — and nothing else can reach it.
// The escaping is devalue's own (`packages/devalue/src/utils.js`): `<` becomes
// `<` so that a value can never close the script element it sits in, and
// the two line terminators JavaScript treats as newlines are escaped too.
//
// Shared references and cycles, which devalue hoists into an IIFE, cannot occur
// in a tree that came back from encoding/json: every object in it is distinct.
//
// This is the temporary half of the seam. `internal/devalue` is growing a real
// `Uneval`; when it lands, the body below becomes a call to it.
func uneval(v any) (string, error) {
	var b strings.Builder
	if err := unevalInto(&b, v); err != nil {
		return "", err
	}
	return b.String(), nil
}

func unevalInto(b *strings.Builder, v any) error {
	switch value := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(value))
	case string:
		unevalString(b, value)
	case float64:
		return unevalNumber(b, value)
	case int:
		b.WriteString(strconv.Itoa(value))
	case json.Number:
		b.WriteString(value.String())
	case []any:
		b.WriteByte('[')
		for i, item := range value {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := unevalInto(b, item); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		// A Go map has no order and a document has to be the same bytes twice
		// in a row, because its ETag is a hash of itself. Kit's own sort is by
		// UTF-16 code unit, which is what JavaScript compares strings by.
		devalue.SortStringsUTF16(keys)
		b.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			unevalKey(b, key)
			b.WriteByte(':')
			if err := unevalInto(b, value[key]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		return fmt.Errorf("skgo: cannot write %T into a document", v)
	}
	return nil
}

func unevalNumber(b *strings.Builder, f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		// devalue writes `NaN` and `Infinity`, both of which evaluate. Nothing
		// that has been through encoding/json can carry one, so a value here
		// means the tree did not come from where it was supposed to.
		return fmt.Errorf("skgo: %v cannot travel in a document", f)
	}
	if f == 0 && math.Signbit(f) {
		b.WriteString("-0")
		return nil
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	// devalue drops the leading zero of a fraction: 0.5 is `.5` and -0.5 is
	// `-.5`. It is the one place its output is not JSON.
	if rest, ok := strings.CutPrefix(s, "0."); ok {
		s = "." + rest
	} else if rest, ok := strings.CutPrefix(s, "-0."); ok {
		s = "-." + rest
	}
	b.WriteString(s)
	return nil
}

// unevalKey writes an object key. devalue writes a bare identifier where it
// can and a quoted string otherwise, and never leaves a reserved word bare.
func unevalKey(b *strings.Builder, key string) {
	if isIdentifier(key) && !reservedWords[key] {
		b.WriteString(key)
		return
	}
	unevalString(b, key)
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || r == '$':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// unevalString quotes a string the way devalue does. The escape set is
// devalue's own: `<` never survives, so that no value can close the script
// element it is written into, and the two Unicode line terminators JavaScript
// treats as newlines are escaped because a raw one is a syntax error.
func unevalString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '<':
			b.WriteString(`\u003C`)
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\u2028':
			b.WriteString(`\u2028`)
		case '\u2029':
			b.WriteString(`\u2029`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(b, `\u%04X`, r)
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}

var reservedWords = map[string]bool{}

func init() {
	for _, word := range strings.Split("do if in for int let new try var byte case char else enum goto long this void with await break catch class const final float short super throw while yield delete double export import native return switch throws typeof boolean default extends finally package private abstract continue debugger function volatile interface protected transient implements instanceof synchronized", " ") {
		reservedWords[word] = true
	}
}
