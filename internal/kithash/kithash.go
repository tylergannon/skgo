// Package kithash reproduces SvelteKit's djb2 string hash.
//
// Ported from `packages/kit/src/utils/hash.js`: the hash walks the string's
// UTF-16 code units backwards, and the result is the unsigned 32-bit value in
// base 36. Remote function ids are `Kit(vite-root-relative posix path)` joined
// to the export name with a slash.
package kithash

import (
	"strconv"
	"unicode/utf16"
)

// Kit hashes a string the way kit's utils/hash.js does.
func Kit(s string) string {
	units := utf16.Encode([]rune(s))

	h := int32(5381)
	for i := len(units) - 1; i >= 0; i-- {
		h = int32(int64(h)*33) ^ int32(units[i])
	}

	return strconv.FormatUint(uint64(uint32(h)), 36)
}
