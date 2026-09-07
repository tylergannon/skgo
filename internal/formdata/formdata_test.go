package formdata

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/tylergannon/polytype/devalue"
)

// The envelopes in this file are goldens taken from kit itself, not built by
// hand here. They were produced by running kit's own `serialize_binary_form` —
// copied verbatim out of the pinned
// ephemeral/inspiration/reference/kit/packages/kit/src/runtime/form-utils.js —
// against the pinned devalue in ephemeral/inspiration/reference/devalue, on
// the Node `mise x -- node` provides. The recipe is in the worklog.
//
// Pinning bytes kit produced is the whole point: a test that built the
// envelope with this package's own encoder would pass no matter how wrong the
// format was.

const (
	// {title: 'Hello', body: 'World'}, no files, no refreshes.
	goldenPlain = "AEYAAAAAAFtbMSw0XSx7InRpdGxlIjoyLCJib2R5IjozfSwiSGVsbG8iLCJXb3JsZCIseyJyZW1vdGVfcmVmcmVzaGVzIjo1fSxbXV0="

	// {count: 42, agree: true, tags: ['a','b'], author: {name: 'Ada'}} with one
	// refresh key — the shape `convert_formdata` produces once it has coerced
	// `n:`- and `b:`-prefixed fields and nested the dotted ones.
	goldenTyped = "AIgAAAAAAFtbMSw5XSx7ImNvdW50IjoyLCJhZ3JlZSI6MywidGFncyI6NCwiYXV0aG9yIjo3fSw0Mix0cnVlLFs1LDZdLCJhIiwiYiIseyJuYW1lIjo4fSwiQWRhIix7InJlbW90ZV9yZWZyZXNoZXMiOjEwfSxbMTFdLCJhYmMxMjMvZ2V0VG9kb3MvIl0="

	// {caption: 'a picture', upload: File('pic.bin', bytes 1..5)}.
	goldenOneFile = "AJYAAAADAFtbMSwxMF0seyJjYXB0aW9uIjoyLCJ1cGxvYWQiOjN9LCJhIHBpY3R1cmUiLFsiRmlsZSIsNF0sWzUsNiw3LDgsOV0sInBpYy5iaW4iLCJhcHBsaWNhdGlvbi9vY3RldC1zdHJlYW0iLDUsMTcwMDAwMDAwMDAwMCwwLHsicmVtb3RlX3JlZnJlc2hlcyI6MTF9LFtdXVswXQECAwQF"

	// {big: File(20 bytes), small: File(2 bytes)}. Kit sorts file *bodies*
	// smallest-first while the offset table stays in header order, so this
	// envelope's table is [2,0] and its body is "ss" then the B's. A decoder
	// that assumed the two orders agreed would hand back each file's bytes
	// under the other one's name.
	goldenTwoFiles = "ALIAAAAFAFtbMSwxNV0seyJiaWciOjIsInNtYWxsIjo5fSxbIkZpbGUiLDNdLFs0LDUsNiw3LDhdLCJiaWcudHh0IiwidGV4dC9wbGFpbiIsMjAsMTcwMDAwMDAwMDAwMSwwLFsiRmlsZSIsMTBdLFsxMSw1LDEyLDEzLDE0XSwic21hbGwudHh0IiwyLDE3MDAwMDAwMDAwMDIsMSx7InJlbW90ZV9yZWZyZXNoZXMiOjE2fSxbXV1bMiwwXXNzQkJCQkJCQkJCQkJCQkJCQkJCQkI="

	// {nothing: File(0 bytes), some: File(3 bytes)} — two files sharing an
	// offset, which the no-gaps/no-overlaps check has to accept.
	goldenEmptyFile = "ALIAAAAFAFtbMSwxNF0seyJub3RoaW5nIjoyLCJzb21lIjo4fSxbIkZpbGUiLDNdLFs0LDUsNiw3LDZdLCJlbXB0eS50eHQiLCJ0ZXh0L3BsYWluIiwwLDE3MDAwMDAwMDAwMDMsWyJGaWxlIiw5XSxbMTAsNSwxMSwxMiwxM10sInNvbWUudHh0IiwzLDE3MDAwMDAwMDAwMDQsMSx7InJlbW90ZV9yZWZyZXNoZXMiOjE1fSxbXV1bMCwwXXh5eg=="
)

func decodeGolden(t *testing.T, golden string) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(golden)
	if err != nil {
		t.Fatalf("decoding golden: %v", err)
	}
	return raw
}

// field pulls one property out of the decoded top-level object.
func field(t *testing.T, data any, name string) any {
	t.Helper()
	obj, ok := data.(*devalue.Object)
	if !ok {
		t.Fatalf("form data is %T, want *devalue.Object", data)
	}
	v, ok := obj.Get(name)
	if !ok {
		t.Fatalf("form data has no field %q (has %v)", name, obj.Keys())
	}
	return v
}

func TestParsePlain(t *testing.T) {
	data, meta, err := Parse(decodeGolden(t, goldenPlain))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := field(t, data, "title"); got != "Hello" {
		t.Errorf("title = %#v, want %q", got, "Hello")
	}
	if got := field(t, data, "body"); got != "World" {
		t.Errorf("body = %#v, want %q", got, "World")
	}
	if len(meta.RemoteRefreshes) != 0 {
		t.Errorf("RemoteRefreshes = %v, want none", meta.RemoteRefreshes)
	}
}

func TestParseCoercedAndNested(t *testing.T) {
	data, meta, err := Parse(decodeGolden(t, goldenTyped))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// The client coerces before it serialises, so a `n:`-prefixed field is
	// already a number on the wire and a `b:`-prefixed one already a boolean.
	if got := field(t, data, "count"); got != float64(42) {
		t.Errorf("count = %#v, want 42", got)
	}
	if got := field(t, data, "agree"); got != true {
		t.Errorf("agree = %#v, want true", got)
	}

	tags, ok := field(t, data, "tags").([]any)
	if !ok || len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("tags = %#v, want [a b]", field(t, data, "tags"))
	}

	author := field(t, data, "author")
	if got := field(t, author, "name"); got != "Ada" {
		t.Errorf("author.name = %#v, want %q", got, "Ada")
	}

	want := []string{"abc123/getTodos/"}
	if len(meta.RemoteRefreshes) != 1 || meta.RemoteRefreshes[0] != want[0] {
		t.Errorf("RemoteRefreshes = %v, want %v", meta.RemoteRefreshes, want)
	}
}

func TestParseOneFile(t *testing.T) {
	data, _, err := Parse(decodeGolden(t, goldenOneFile))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := field(t, data, "caption"); got != "a picture" {
		t.Errorf("caption = %#v", got)
	}

	file, ok := field(t, data, "upload").(File)
	if !ok {
		t.Fatalf("upload is %T, want File", field(t, data, "upload"))
	}
	if file.Name != "pic.bin" {
		t.Errorf("Name = %q, want %q", file.Name, "pic.bin")
	}
	if file.Type != "application/octet-stream" {
		t.Errorf("Type = %q", file.Type)
	}
	if file.LastModified != 1700000000000 {
		t.Errorf("LastModified = %d", file.LastModified)
	}
	// The bytes are the literal ones the golden was built from, written here
	// rather than read back off the envelope.
	if want := []byte{1, 2, 3, 4, 5}; string(file.Data) != string(want) {
		t.Errorf("Data = %v, want %v", file.Data, want)
	}
}

// TestParseTwoFilesKeepsBytesWithTheirOwnField is the case that catches the
// obvious wrong implementation. Kit writes the file bodies smallest-first but
// leaves the offset table in the order the files appear in the header, so
// walking the bodies in header order silently swaps the two files' contents.
func TestParseTwoFilesKeepsBytesWithTheirOwnField(t *testing.T) {
	data, _, err := Parse(decodeGolden(t, goldenTwoFiles))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	big, ok := field(t, data, "big").(File)
	if !ok {
		t.Fatalf("big is %T, want File", field(t, data, "big"))
	}
	small, ok := field(t, data, "small").(File)
	if !ok {
		t.Fatalf("small is %T, want File", field(t, data, "small"))
	}

	if big.Name != "big.txt" || small.Name != "small.txt" {
		t.Fatalf("names = %q, %q", big.Name, small.Name)
	}
	if want := strings.Repeat("B", 20); string(big.Data) != want {
		t.Errorf("big.Data = %q, want %q", big.Data, want)
	}
	if string(small.Data) != "ss" {
		t.Errorf("small.Data = %q, want %q", small.Data, "ss")
	}
}

func TestParseEmptyFile(t *testing.T) {
	data, _, err := Parse(decodeGolden(t, goldenEmptyFile))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	nothing := field(t, data, "nothing").(File)
	some := field(t, data, "some").(File)

	if nothing.Name != "empty.txt" || len(nothing.Data) != 0 {
		t.Errorf("nothing = %q/%q", nothing.Name, nothing.Data)
	}
	if some.Name != "some.txt" || string(some.Data) != "xyz" {
		t.Errorf("some = %q/%q", some.Name, some.Data)
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	good := decodeGolden(t, goldenOneFile)

	corrupt := func(fn func([]byte) []byte) []byte {
		clone := make([]byte, len(good))
		copy(clone, good)
		return fn(clone)
	}

	cases := []struct {
		name string
		body []byte
	}{
		{"empty", nil},
		{"truncated prologue", good[:5]},
		{"wrong version", corrupt(func(b []byte) []byte { b[0] = 1; return b })},
		{"header longer than body", corrupt(func(b []byte) []byte {
			b[1], b[2], b[3], b[4] = 0xff, 0xff, 0, 0
			return b
		})},
		{"offset table longer than body", corrupt(func(b []byte) []byte {
			b[5], b[6] = 0xff, 0xff
			return b
		})},
		{"body truncated below the file", good[:len(good)-3]},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := Parse(tc.body); err == nil {
				t.Fatal("Parse accepted a malformed envelope")
			} else if !errors.Is(err, ErrBadRequest) {
				t.Fatalf("error %v is not an ErrBadRequest", err)
			}
		})
	}
}
