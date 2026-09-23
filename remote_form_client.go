// The Go client for SvelteKit's enhanced remote Form protocol.
package skgo

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/tylergannon/polytype/devalue"
)

// FormClient sends enhanced Form submissions to the same endpoint used by Kit.
// HTTPClient may use a custom Transport (including a Unix socket dialer).
// Each call makes exactly one HTTP request; a lost reply is never replayed.
type FormClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// SubmitForm performs one typed Form submission. The generated client supplies
// the endpoint identity and its result decoder; callers never handle the wire.
func SubmitForm[In, Out any](ctx context.Context, client FormClient, id string, input In, decode func(any) (Out, error)) (Out, error) {
	var zero Out
	data, err := formClientObject(reflect.ValueOf(input))
	if err != nil {
		return zero, err
	}
	header, err := devalue.Stringify([]any{data, devalue.NewObject("remote_refreshes", []any{})})
	if err != nil {
		return zero, fmt.Errorf("skgo: encode form: %w", err)
	}
	if uint64(len(header)) > uint64(^uint32(0)) {
		return zero, fmt.Errorf("skgo: form header too large")
	}
	body := make([]byte, 7+len(header))
	binary.LittleEndian.PutUint32(body[1:5], uint32(len(header)))
	copy(body[7:], header)
	base := strings.TrimRight(client.BaseURL, "/")
	if base == "" {
		return zero, fmt.Errorf("skgo: FormClient.BaseURL is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/_app/remote/"+id, bytes.NewReader(body))
	if err != nil {
		return zero, err
	}
	req.Header.Set("Content-Type", "application/x-sveltekit-formdata")
	req.Header.Set("Origin", req.URL.Scheme+"://"+req.URL.Host)
	httpClient := client.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	// A 307/308 response would make http.Client replay this POST because the
	// bytes.Reader body is rewindable. Keep the caller's client untouched and
	// let every redirect be a returned response, never a second mutation.
	oneShot := *httpClient
	oneShot.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := oneShot.Do(req)
	if err != nil {
		return zero, err
	}
	defer res.Body.Close()
	var envelope struct {
		Type  string     `json:"type"`
		Data  string     `json:"data"`
		Error *HTTPError `json:"error"`
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil {
		return zero, fmt.Errorf("skgo: read Form response (HTTP %d): %w", res.StatusCode, err)
	}
	if len(raw) > 1<<20 {
		return zero, fmt.Errorf("skgo: Form response too large (HTTP %d)", res.StatusCode)
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return zero, fmt.Errorf("skgo: decode Form response (HTTP %d): %w", res.StatusCode, err)
	}
	if envelope.Type == "error" && envelope.Error != nil {
		return zero, envelope.Error
	}
	if res.StatusCode != http.StatusOK || envelope.Type != "result" || envelope.Data == "" || envelope.Error != nil {
		return zero, fmt.Errorf("skgo: invalid Form response (HTTP %d, type %q)", res.StatusCode, envelope.Type)
	}
	tree, err := devalue.Parse(envelope.Data, nil)
	if err != nil {
		return zero, fmt.Errorf("skgo: decode Form result: %w", err)
	}
	root, ok := tree.(*devalue.Object)
	if !ok {
		return zero, fmt.Errorf("skgo: Form result is not an object")
	}
	value, ok := root.Get("_")
	if !ok {
		return zero, fmt.Errorf("skgo: Form result has no submission")
	}
	submission, ok := value.(*devalue.Object)
	if !ok {
		return zero, fmt.Errorf("skgo: malformed Form submission")
	}
	flag, ok := submission.Get("submission")
	if !ok || flag != true {
		return zero, fmt.Errorf("skgo: malformed Form submission flag")
	}
	if rawIssues, found := submission.Get("issues"); found {
		items, ok := rawIssues.([]any)
		if !ok {
			return zero, fmt.Errorf("skgo: malformed Form issues")
		}
		invalid := &Invalid{}
		for _, item := range items {
			obj, ok := item.(*devalue.Object)
			if !ok {
				return zero, fmt.Errorf("skgo: malformed Form issue")
			}
			name, nameOK := obj.Get("name")
			message, messageOK := obj.Get("message")
			field, fieldOK := name.(string)
			msg, msgOK := message.(string)
			if !nameOK || !messageOK || !fieldOK || !msgOK {
				return zero, fmt.Errorf("skgo: malformed Form issue")
			}
			invalid.Issues = append(invalid.Issues, Issue{Field: field, Message: msg})
		}
		return zero, invalid
	}
	result, ok := submission.Get("result")
	if !ok {
		return zero, fmt.Errorf("skgo: Form submission has no result")
	}
	out, err := decode(result)
	if err != nil {
		return zero, fmt.Errorf("skgo: decode Form result: %w", err)
	}
	return out, nil
}

func formClientObject(value reflect.Value) (*devalue.Object, error) {
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, fmt.Errorf("skgo: nil Form input")
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return nil, fmt.Errorf("skgo: Form input must be a struct")
	}
	object := devalue.NewObject()
	for i := 0; i < value.NumField(); i++ {
		field := value.Type().Field(i)
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		v := value.Field(i)
		if isClientOptional(v.Type()) {
			if !v.FieldByName("Present").Bool() {
				continue
			}
			v = v.FieldByName("Value")
		}
		switch v.Kind() {
		case reflect.String:
			object.Set(name, v.String())
		case reflect.Bool:
			object.Set(name, v.Bool())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n := v.Int()
			if n < -(1<<53)+1 || n > (1<<53)-1 {
				return nil, fmt.Errorf("skgo: Form field %s exceeds JavaScript's safe integer range", name)
			}
			object.Set(name, float64(n))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			n := v.Uint()
			if n > (1<<53)-1 {
				return nil, fmt.Errorf("skgo: Form field %s exceeds JavaScript's safe integer range", name)
			}
			object.Set(name, float64(n))
		case reflect.Float32, reflect.Float64:
			object.Set(name, v.Float())
		default:
			return nil, fmt.Errorf("skgo: unsupported Form field %s (%s)", name, v.Type())
		}
	}
	return object, nil
}

func isClientOptional(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && t.PkgPath() == "github.com/tylergannon/polytype" && strings.HasPrefix(t.Name(), "Optional[")
}
