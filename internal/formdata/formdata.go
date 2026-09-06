// Package formdata decodes the body kit's enhanced form client posts.
//
// A SvelteKit form submitted from a hydrated page does not send
// multipart/form-data. `form.svelte.js` converts the FormData into a POJO
// first — coercing `n:`/`b:` prefixed fields to numbers and booleans and
// nesting dotted names — and then posts that POJO in kit's own binary
// envelope, `application/x-sveltekit-formdata`. Files are lifted out of the
// devalue header and appended raw, so a large upload never has to be
// base64'd.
//
// The envelope is defined in `runtime/form-utils.js`:
//
//	1 byte  format version (0)
//	4 bytes header length, little-endian u32
//	2 bytes file offset table length, little-endian u16
//	N bytes header: devalue.stringify([data, meta])
//	M bytes file offset table: JSON array of offsets, or empty when no files
//	        file bodies, concatenated, smallest first
//
// The offsets are relative to the end of the table and are listed in the order
// the files appear in the header, not the order they appear in the body.
package formdata

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/tylergannon/skgo/internal/devalue"
)

// ContentType is the media type kit's enhanced client posts a form as.
const ContentType = "application/x-sveltekit-formdata"

// version is the only envelope version kit emits.
const version = 0

// headerBytes is the fixed-size prologue: version, header length, table length.
const headerBytes = 1 + 4 + 2

// fileTag is devalue's built-in tag for a File. Kit's form reducer registers a
// reviver under exactly this name, so the payload arrives tagged with it —
// note this is *not* the `__skraf` encoding remotearg uses for a command
// argument, which carries the bytes inline as an ArrayBuffer.
const fileTag = "File"

// File is one uploaded file, with its bytes already resolved from the body.
type File struct {
	// Name is the file's name as the browser reported it.
	Name string
	// Type is its MIME type, which the browser guesses and a caller must not
	// trust.
	Type string
	// LastModified is the browser's mtime, in milliseconds since the epoch.
	LastModified int64
	// Data is the file's contents.
	Data []byte
}

// Size is the file's length in bytes.
func (f File) Size() int { return len(f.Data) }

// Meta is the second element of the header, the submission's metadata. Kit
// sends only the single-flight refresh keys.
type Meta struct {
	// RemoteRefreshes are the `<hash>/<name>/<payload>` keys the client asked
	// the server to resolve in the same flight.
	RemoteRefreshes []string
	// ValidateOnly asks for the schema's verdict on the data without running
	// the handler. Kit's client sends it from `form.validate()`, which runs as
	// the visitor types.
	ValidateOnly bool
}

// ErrBadRequest is the class every malformed-body error belongs to. The body
// came from the client, so a failure to read it is a 400 and never a 500.
var ErrBadRequest = errors.New("skgo: malformed form submission")

func badRequest(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrBadRequest}, args...)...)
}

// Parse decodes a binary form body. data is the POJO kit's client built from
// the form's fields — a *devalue.Object at the top, with File values wherever
// the form carried a file.
func Parse(body []byte) (data any, meta Meta, err error) {
	if len(body) < headerBytes {
		return nil, Meta{}, badRequest("too short")
	}
	if body[0] != version {
		return nil, Meta{}, badRequest("got version %d, expected version %d", body[0], version)
	}

	headerLen := binary.LittleEndian.Uint32(body[1:5])
	offsetsLen := binary.LittleEndian.Uint16(body[5:7])

	// The lengths are read from the envelope rather than from Content-Length,
	// which a proxy may strip or rewrite. They are attacker-controlled, so
	// every span is checked against the body before it is sliced.
	headerEnd := uint64(headerBytes) + uint64(headerLen)
	if headerEnd > uint64(len(body)) {
		return nil, Meta{}, badRequest("data too short")
	}
	tableEnd := headerEnd + uint64(offsetsLen)
	if tableEnd > uint64(len(body)) {
		return nil, Meta{}, badRequest("file offset table too short")
	}

	offsets, err := parseOffsets(body[headerEnd:tableEnd])
	if err != nil {
		return nil, Meta{}, err
	}

	files := &fileTable{offsets: offsets, start: tableEnd, body: body}
	parsed, err := devalue.Parse(string(body[headerBytes:headerEnd]), map[string]func(any) (any, error){
		fileTag: files.revive,
	})
	if err != nil {
		return nil, Meta{}, badRequest("%v", err)
	}
	if err := files.check(); err != nil {
		return nil, Meta{}, err
	}

	// The header is `[data, meta]`; anything else is not a form submission.
	pair, ok := parsed.([]any)
	if !ok || len(pair) != 2 {
		return nil, Meta{}, badRequest("header is not a [data, meta] pair")
	}
	return pair[0], readMeta(pair[1]), nil
}

// parseOffsets reads the file offset table. An empty table means the
// submission carried no files, which is the common case.
func parseOffsets(raw []byte) ([]int64, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var offsets []int64
	if err := json.Unmarshal(raw, &offsets); err != nil {
		return nil, badRequest("invalid file offset table")
	}
	for _, n := range offsets {
		if n < 0 {
			return nil, badRequest("invalid file offset table")
		}
	}
	return offsets, nil
}

// fileTable resolves a file's bytes out of the body and enforces the two rules
// kit enforces: an offset table entry is claimed at most once, and the file
// bodies tile the tail of the request exactly — no gaps, no overlaps. Both
// exist so a crafted header cannot make one file's bytes appear inside
// another, or read a span the sender never sent.
type fileTable struct {
	offsets []int64
	claimed []bool
	start   uint64
	body    []byte
	spans   []span
}

type span struct{ offset, size uint64 }

func (t *fileTable) revive(v any) (any, error) {
	meta, ok := v.([]any)
	if !ok || len(meta) != 5 {
		return nil, badRequest("invalid file metadata")
	}
	name, nameOK := meta[0].(string)
	mime, mimeOK := meta[1].(string)
	size, sizeOK := meta[2].(float64)
	modified, modifiedOK := meta[3].(float64)
	index, indexOK := meta[4].(float64)
	if !nameOK || !mimeOK || !sizeOK || !modifiedOK || !indexOK {
		return nil, badRequest("invalid file metadata")
	}
	if size < 0 || index < 0 || index != float64(int(index)) {
		return nil, badRequest("invalid file metadata")
	}

	i := int(index)
	if i >= len(t.offsets) {
		return nil, badRequest("file offset table index out of range")
	}
	if t.claimed == nil {
		t.claimed = make([]bool, len(t.offsets))
	}
	if t.claimed[i] {
		return nil, badRequest("duplicate file offset table index")
	}
	t.claimed[i] = true

	offset := t.start + uint64(t.offsets[i])
	length := uint64(size)
	if offset > uint64(len(t.body)) || offset+length > uint64(len(t.body)) {
		return nil, badRequest("file data out of range")
	}
	t.spans = append(t.spans, span{offset: offset, size: length})

	// The bytes are copied because the caller may hold the file for longer
	// than the request body buffer lives.
	data := make([]byte, length)
	copy(data, t.body[offset:offset+length])

	return File{Name: name, Type: mime, LastModified: int64(modified), Data: data}, nil
}

// check enforces that the file bodies tile the tail of the request exactly.
func (t *fileTable) check() error {
	sort.Slice(t.spans, func(i, j int) bool {
		if t.spans[i].offset != t.spans[j].offset {
			return t.spans[i].offset < t.spans[j].offset
		}
		return t.spans[i].size < t.spans[j].size
	})
	for i := 1; i < len(t.spans); i++ {
		end := t.spans[i-1].offset + t.spans[i-1].size
		if end < t.spans[i].offset {
			return badRequest("gaps in file data")
		}
		if end > t.spans[i].offset {
			return badRequest("overlapping file data")
		}
	}
	return nil
}

// readMeta pulls the refresh keys out of the header's second element. A
// missing or malformed entry means no refreshes, which is what kit's client
// sends when the form asked for none.
func readMeta(v any) Meta {
	obj, ok := v.(*devalue.Object)
	if !ok {
		return Meta{}
	}
	var meta Meta
	if raw, ok := obj.Get("validate_only"); ok {
		meta.ValidateOnly, _ = raw.(bool)
	}
	raw, ok := obj.Get("remote_refreshes")
	if !ok {
		return meta
	}
	list, ok := raw.([]any)
	if !ok {
		return meta
	}
	meta.RemoteRefreshes = make([]string, 0, len(list))
	for _, item := range list {
		if key, ok := item.(string); ok {
			meta.RemoteRefreshes = append(meta.RemoteRefreshes, key)
		}
	}
	return meta
}
