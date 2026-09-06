package kithash

import (
	"strconv"
	"testing"
	"unicode/utf16"
)

func TestKitGoldens(t *testing.T) {
	// Paths and hashes observed from a running kit build.
	cases := []struct {
		path string
		want string
	}{
		{"src/lib/todos.remote.ts", "worolc"},
		{"src/routes/data.remote.js", "mxe8u8"},
		{"src/lib/a.remote.ts", "txkpmq"},
		{"src/lib/document.remote.ts", "2rsbgs"},
		{"src/lib/secondary.remote.ts", "16ghqs9"},
	}

	for _, c := range cases {
		if got := Kit(c.path); got != c.want {
			t.Errorf("Kit(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestKitEmpty(t *testing.T) {
	// The seed, 5381, in base 36.
	if got, want := Kit(""), strconv.FormatUint(5381, 36); got != want {
		t.Errorf("Kit(%q) = %q, want %q", "", got, want)
	}
}

// reference is the algorithm spelled out over explicit UTF-16 code units, so
// that Kit's use of utf16.Encode is pinned rather than assumed.
func reference(units []uint16) string {
	h := int32(5381)
	for i := len(units) - 1; i >= 0; i-- {
		h = int32(int64(h)*33) ^ int32(units[i])
	}
	return strconv.FormatUint(uint64(uint32(h)), 36)
}

func TestKitUsesUTF16CodeUnits(t *testing.T) {
	const astral = "a\U0001D306b" // U+1D306 is a surrogate pair in UTF-16

	if got, want := Kit(astral), reference([]uint16{'a', 0xD834, 0xDF06, 'b'}); got != want {
		t.Errorf("Kit(astral) = %q, want %q", got, want)
	}

	// Hashing runes rather than code units would differ; prove the astral
	// character really does encode to two units.
	if n := len(utf16.Encode([]rune(astral))); n != 4 {
		t.Fatalf("expected 4 UTF-16 code units, got %d", n)
	}
}

func TestKitIsOrderSensitive(t *testing.T) {
	if Kit("ab") == Kit("ba") {
		t.Error("hash is not order sensitive")
	}
}
