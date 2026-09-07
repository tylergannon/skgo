package formdata

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/tylergannon/polytype/devalue"
)

// Convert turns the controls of an ordinary <form> submission into the same
// POJO kit's client would have built from them.
//
// It is `convert_formdata` from `packages/kit/src/runtime/form-utils.js`, and
// it exists for the submission kit's client did not enhance: a page with
// scripting off posts `application/x-www-form-urlencoded` or
// `multipart/form-data` to the page's own URL, and kit answers that by calling
// exactly this function on the FormData before it runs the form's handler
// (`deserialize_binary_form`, the branch taken when the content type is not
// kit's binary envelope).
//
// So the handler cannot tell the two submissions apart, which is the whole
// point: the Go function that answers an enhanced submission answers this one
// unchanged.
func Convert(formID string, entries []Entry) (any, error) {
	result := devalue.NewObject()

	// FormData iterates its keys in insertion order and `getAll` gathers every
	// value under one of them, so the controls are grouped here the same way
	// rather than by sorting.
	order := make([]string, 0, len(entries))
	grouped := map[string][]Entry{}
	for _, entry := range entries {
		if _, seen := grouped[entry.Name]; !seen {
			order = append(order, entry.Name)
		}
		grouped[entry.Name] = append(grouped[entry.Name], entry)
	}

	for _, name := range order {
		field, err := ParseFormKey(formID, name)
		if err != nil {
			return nil, err
		}

		// An empty `<input type="file">` submits a nameless zero-length file,
		// bizarrely, and kit drops it here rather than letting it reach the
		// handler as a File nobody chose.
		values := make([]Entry, 0, len(grouped[name]))
		for _, entry := range grouped[name] {
			if entry.File != nil && entry.File.Name == "" && entry.File.Size() == 0 {
				continue
			}
			values = append(values, entry)
		}

		if len(values) == 0 && !field.IsArray {
			continue
		}
		if len(values) > 1 && !field.IsArray {
			return nil, badRequest("form cannot contain duplicated keys — %q has %d values", field.Name, len(values))
		}

		var value any
		if field.IsArray {
			items := make([]any, 0, len(values))
			for _, entry := range values {
				items = append(items, coerce(field.Type, entry))
			}
			value = items
		} else {
			value = coerce(field.Type, values[0])
		}

		if err := setNested(result, field, value); err != nil {
			return nil, err
		}
	}

	return settle(result), nil
}

// Entry is one control of a submission: a name and either text or a file.
type Entry struct {
	// Name is the control's `name` attribute, still carrying the type prefix
	// and the form id `fields.<path>.as(...)` put in it.
	Name string
	// Value is the text the control submitted, for a control that is not a
	// file input.
	Value string
	// File is the uploaded file, for a control that is.
	File *File
}

// Field is a control's name taken apart: kit's `parse_form_key`.
type Field struct {
	// Name is the field's path within the form's argument — "email",
	// "author.name", "tags[0]" — with the type prefix and the form id removed.
	Name string
	// Type is the coercion the control declared: "number", "boolean", or "".
	Type string
	// IsArray reports the `[]` suffix, which is how a control says it is one
	// of several values under one name.
	IsArray bool
}

// ParseFormKey separates a control's path from the metadata encoded in its
// name. A name that does not end in the form's own id was not created by
// `form.fields.<path>.as(...)`, and kit refuses it — the check is what stops a
// submission from naming a field of some other form.
func ParseFormKey(formID, key string) (Field, error) {
	suffix := "/" + formID
	if !strings.HasSuffix(key, suffix) {
		return Field{}, badRequest("form contained a field that wasn't created with form.fields.as(...): %s", key)
	}
	name := strings.TrimSuffix(key, suffix)

	typ := ""
	switch {
	case strings.HasPrefix(name, "n:"):
		name, typ = name[2:], "number"
	case strings.HasPrefix(name, "b:"):
		name, typ = name[2:], "boolean"
	}

	isArray := strings.HasSuffix(name, "[]")
	if isArray {
		name = strings.TrimSuffix(name, "[]")
	}

	return Field{Name: name, Type: typ, IsArray: isArray}, nil
}

// coerce is kit's `coerce_form_value`: a control declared `as('number')` sends
// text and arrives as a number, one declared `as('checkbox')` arrives as a
// boolean, and everything else arrives as it was typed.
func coerce(typ string, entry Entry) any {
	if entry.File != nil {
		return *entry.File
	}
	switch typ {
	case "number":
		if entry.Value == "" {
			// An empty number input is absent rather than zero. Kit says so
			// with `undefined`, and Decode leaves an undefined field alone.
			return devalue.Undefined
		}
		f, err := strconv.ParseFloat(entry.Value, 64)
		if err != nil {
			// `parseFloat` gives NaN, and NaN is what kit hands the handler.
			return nan()
		}
		return f
	case "boolean":
		return entry.Value == "on"
	default:
		return entry.Value
	}
}

func nan() float64 {
	var zero float64
	return zero / zero
}

// pathPattern is kit's `path_regex`. A name that does not match it never came
// from the field proxy, and walking it would be a way to reach places in the
// argument the form does not describe.
var pathPattern = regexp.MustCompile(`^[a-zA-Z_$]\w*(\.[a-zA-Z_$]\w*|\[\d+\])*$`)

var pathSeparators = regexp.MustCompile(`[.\[\]]`)

// splitPath is kit's `split_path`: "author.name" is ["author","name"] and
// "tags[0]" is ["tags","0"].
func splitPath(path string) ([]string, error) {
	if !pathPattern.MatchString(path) {
		return nil, badRequest("invalid path %s", path)
	}
	parts := pathSeparators.Split(path, -1)
	out := parts[:0]
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out, nil
}

// arrayNode is a JavaScript array while the tree is being built. It is a
// pointer so that a nested array can grow without its parent having to be
// rewritten, and it becomes a plain []any once the whole submission is in.
type arrayNode struct{ items []any }

func (a *arrayNode) get(i int) (any, bool) {
	if i < 0 || i >= len(a.items) {
		return nil, false
	}
	return a.items[i], a.items[i] != devalue.Hole
}

// set writes one element, extending the array with holes the way assigning
// past the end of a JavaScript array does.
func (a *arrayNode) set(i int, v any) {
	for len(a.items) <= i {
		a.items = append(a.items, devalue.Hole)
	}
	a.items[i] = v
}

// setNested is kit's `set_nested_value`/`deep_set`: it walks the path,
// creating an array where the next segment is a number and an object where it
// is not, and refuses a segment that would rewrite a prototype.
func setNested(root *devalue.Object, field Field, value any) error {
	keys, err := splitPath(field.Name)
	if err != nil {
		return err
	}

	var current any = root
	for i := 0; i < len(keys)-1; i++ {
		key := keys[i]
		if err := checkPollution(key); err != nil {
			return err
		}
		wantArray := isIndex(keys[i+1])

		inner, exists := child(current, key)
		if exists && inner != nil {
			if _, isArray := inner.(*arrayNode); isArray != wantArray {
				return badRequest("invalid array key %s", keys[i+1])
			}
		} else {
			if wantArray {
				inner = &arrayNode{}
			} else {
				inner = devalue.NewObject()
			}
			if err := setChild(current, key, inner); err != nil {
				return err
			}
		}
		current = inner
	}

	last := keys[len(keys)-1]
	if err := checkPollution(last); err != nil {
		return err
	}
	return setChild(current, last, value)
}

func isIndex(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func checkPollution(key string) error {
	switch key {
	case "__proto__", "constructor", "prototype":
		return badRequest("invalid key %q", key)
	}
	return nil
}

func child(node any, key string) (any, bool) {
	switch container := node.(type) {
	case *devalue.Object:
		return container.Get(key)
	case *arrayNode:
		i, err := strconv.Atoi(key)
		if err != nil {
			return nil, false
		}
		return container.get(i)
	default:
		return nil, false
	}
}

func setChild(node any, key string, value any) error {
	switch container := node.(type) {
	case *devalue.Object:
		container.Set(key, value)
		return nil
	case *arrayNode:
		i, err := strconv.Atoi(key)
		if err != nil || i < 0 {
			return badRequest("invalid array index %q", key)
		}
		container.set(i, value)
		return nil
	default:
		return badRequest("cannot set %q on %T", key, node)
	}
}

// settle replaces every arrayNode with the []any the rest of skgo reads,
// leaving the tree in exactly the shape ParseWith produces for an enhanced
// submission.
func settle(node any) any {
	switch container := node.(type) {
	case *devalue.Object:
		for _, key := range container.Keys() {
			v, _ := container.Get(key)
			container.Set(key, settle(v))
		}
		return container
	case *arrayNode:
		out := make([]any, len(container.items))
		for i, item := range container.items {
			out[i] = settle(item)
		}
		return out
	default:
		return node
	}
}

// ConvertRaw is the `input` half of kit's `handle_issues`: the same nesting as
// [Convert] over the same names, and none of the rest of it.
//
// Nothing is coerced — a control declared `as('number')` comes back as the text
// that was typed — nothing is dropped for being empty, and a duplicate name is
// not an error. That is deliberate on kit's part: this object refills the
// controls of a form the visitor is about to see again, and a control holds
// text.
func ConvertRaw(formID string, entries []Entry) (*devalue.Object, error) {
	result := devalue.NewObject()

	order := make([]string, 0, len(entries))
	grouped := map[string][]Entry{}
	for _, entry := range entries {
		if _, seen := grouped[entry.Name]; !seen {
			order = append(order, entry.Name)
		}
		grouped[entry.Name] = append(grouped[entry.Name], entry)
	}

	for _, name := range order {
		field, err := ParseFormKey(formID, name)
		if err != nil {
			return nil, err
		}

		values := make([]any, 0, len(grouped[name]))
		for _, entry := range grouped[name] {
			if entry.File != nil {
				// A file is not a string, and kit's filter keeps only strings.
				continue
			}
			values = append(values, entry.Value)
		}

		var value any
		if field.IsArray {
			value = values
		} else if len(values) == 0 {
			// `values[0]` of an empty array. Kit sets it rather than skipping
			// the key, and Decode leaves an undefined field alone.
			value = devalue.Undefined
		} else {
			value = values[0]
		}

		if err := setNested(result, field, value); err != nil {
			return nil, err
		}
	}

	settled, _ := settle(result).(*devalue.Object)
	return settled, nil
}
