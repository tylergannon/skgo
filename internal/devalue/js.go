package devalue

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// quoteJS is devalue's `stringify_string`: a JSON string literal in which `<`
// becomes \u003C so the output is safe inside a <script> element, and U+2028 /
// U+2029 are escaped because they are line terminators in JavaScript source
// but not in JSON.
//
// It walks bytes rather than runes so that a lone surrogate — which a Go string
// can only carry as WTF-8, and which devalue emits verbatim — survives
// byte-for-byte instead of being replaced with U+FFFD.
func quoteJS(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			switch c {
			case '"':
				b.WriteString(`\"`)
			case '\\':
				b.WriteString(`\\`)
			case '<':
				b.WriteString(`\u003C`)
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			case '\b':
				b.WriteString(`\b`)
			case '\f':
				b.WriteString(`\f`)
			default:
				if c < ' ' {
					const hex = "0123456789abcdef"
					b.WriteString(`\u00`)
					b.WriteByte(hex[(c>>4)&0xf])
					b.WriteByte(hex[c&0xf])
				} else {
					b.WriteByte(c)
				}
			}
			i++
			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			// Not valid UTF-8 — a WTF-8 surrogate, or arbitrary bytes. Pass it
			// through as devalue passes a lone surrogate through.
			b.WriteByte(c)
		case r == '\u2028':
			b.WriteString(`\u2028`)
		case r == '\u2029':
			b.WriteString(`\u2029`)
		default:
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	b.WriteByte('"')
	return b.String()
}

// isIdentifier is devalue's /^[_$a-zA-Z][_$a-zA-Z0-9]*$/. It is written out
// rather than compiled as a regexp because it runs once per property.
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		case i > 0 && c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return true
}

// safeKey renders an object-literal key: bare when it is an identifier, quoted
// otherwise.
func safeKey(key string) string {
	if isIdentifier(key) {
		return key
	}
	return quoteJS(key)
}

// safeProp renders a property access on a hoisted name: `.foo` or `["x-y"]`.
func safeProp(key string) string {
	if isIdentifier(key) {
		return "." + key
	}
	return "[" + quoteJS(key) + "]"
}

// nameChars is devalue's alphabet for hoisted parameter names.
const nameChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_$"

// reservedWords is devalue's `reserved` regexp, which covers the ECMAScript
// reserved words plus the ones reserved by older editions and by Java-derived
// grammars. A generated name that lands on one gets a "0" suffix.
var reservedWords = map[string]bool{
	"do": true, "if": true, "in": true, "for": true, "int": true, "let": true,
	"new": true, "try": true, "var": true, "byte": true, "case": true,
	"char": true, "else": true, "enum": true, "goto": true, "long": true,
	"this": true, "void": true, "with": true, "await": true, "break": true,
	"catch": true, "class": true, "const": true, "final": true, "float": true,
	"short": true, "super": true, "throw": true, "while": true, "yield": true,
	"delete": true, "double": true, "export": true, "import": true,
	"native": true, "return": true, "switch": true, "throws": true,
	"typeof": true, "boolean": true, "default": true, "extends": true,
	"finally": true, "package": true, "private": true, "abstract": true,
	"continue": true, "debugger": true, "function": true, "volatile": true,
	"interface": true, "protected": true, "transient": true,
	"implements": true, "instanceof": true, "synchronized": true,
}

// getName is devalue's `get_name`: a bijective base-54 numeral over nameChars,
// suffixed with "0" when it collides with a reserved word.
func getName(num int) string {
	var name string
	for {
		name = string(nameChars[num%len(nameChars)]) + name
		num = num/len(nameChars) - 1
		if num < 0 {
			break
		}
	}
	if reservedWords[name] {
		return name + "0"
	}
	return name
}

// formatNumber implements ECMAScript's Number::toString for base 10, which
// differs from every Go format verb around the exponent thresholds: JavaScript
// switches to exponential notation below 1e-6 and at or above 1e21.
func formatNumber(f float64) string {
	if f == 0 {
		return "0"
	}
	if math.IsNaN(f) {
		return "NaN"
	}
	if math.IsInf(f, 1) {
		return "Infinity"
	}
	if math.IsInf(f, -1) {
		return "-Infinity"
	}

	neg := f < 0
	if neg {
		f = -f
	}

	// Shortest round-tripping decimal, as digits and a decimal exponent.
	e := strconv.FormatFloat(f, 'e', -1, 64)
	mantissa, exponent, _ := strings.Cut(e, "e")
	exp, err := strconv.Atoi(exponent)
	if err != nil {
		return e
	}

	digits := strings.Replace(mantissa, ".", "", 1)
	k := len(digits)
	n := exp + 1 // position of the decimal point

	var s string
	switch {
	case k <= n && n <= 21:
		s = digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		s = digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		s = "0." + strings.Repeat("0", -n) + digits
	default:
		m := digits
		if k > 1 {
			m = digits[:1] + "." + digits[1:]
		}
		if n-1 >= 0 {
			s = m + "e+" + strconv.Itoa(n-1)
		} else {
			s = m + "e-" + strconv.Itoa(1-n)
		}
	}

	if neg {
		return "-" + s
	}
	return s
}

// maxArrayIndex is the largest valid JavaScript array index.
const maxArrayIndex = 1<<32 - 2

// isArrayIndexString reports whether a property name is a canonical array
// index, which JavaScript hoists ahead of ordinary string keys.
func isArrayIndexString(s string) bool {
	if s == "" || len(s) > 10 {
		return false
	}
	if len(s) > 1 && s[0] == '0' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return err == nil && n <= maxArrayIndex
}

// propertyOrder returns keys in JavaScript property order: canonical array
// indices ascending, then the remaining keys in insertion order. This is the
// order `Object.keys` yields, and therefore the order devalue emits.
func propertyOrder(keys []string) []string {
	var indices, rest []string
	for _, k := range keys {
		if isArrayIndexString(k) {
			indices = append(indices, k)
		} else {
			rest = append(rest, k)
		}
	}
	if len(indices) == 0 {
		return rest
	}
	sort.Slice(indices, func(i, j int) bool {
		a, _ := strconv.ParseUint(indices[i], 10, 64)
		b, _ := strconv.ParseUint(indices[j], 10, 64)
		return a < b
	})
	return append(indices, rest...)
}
