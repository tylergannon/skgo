package formdata

import (
	"errors"
	"strings"
	"testing"
)

// The goldens are real kit envelopes (see formdata_test.go); the values
// asserted here are the ones the generator script put into them, written out
// literally rather than read back off the same tree the decoder walked.

func TestDecodeIntoStruct(t *testing.T) {
	data, _, err := Parse(decodeGolden(t, goldenPlain))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	var got struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := Decode(data, &got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Title != "Hello" || got.Body != "World" {
		t.Errorf("got %+v, want {Hello World}", got)
	}
}

func TestDecodeCoercedNestedAndArray(t *testing.T) {
	data, _, err := Parse(decodeGolden(t, goldenTyped))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	var got struct {
		Count  int      `json:"count"`
		Agree  bool     `json:"agree"`
		Tags   []string `json:"tags"`
		Author struct {
			Name string `json:"name"`
		} `json:"author"`
	}
	if err := Decode(data, &got); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if got.Count != 42 {
		t.Errorf("Count = %d, want 42", got.Count)
	}
	if !got.Agree {
		t.Error("Agree = false, want true")
	}
	if strings.Join(got.Tags, ",") != "a,b" {
		t.Errorf("Tags = %v, want [a b]", got.Tags)
	}
	if got.Author.Name != "Ada" {
		t.Errorf("Author.Name = %q, want %q", got.Author.Name, "Ada")
	}
}

// TestDecodeFileBytesReachGo is the whole reason this decoder exists: an
// encoding/json round-trip cannot carry a File, so the bytes have to arrive by
// another route.
func TestDecodeFileBytesReachGo(t *testing.T) {
	data, _, err := Parse(decodeGolden(t, goldenOneFile))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	var got struct {
		Caption string `json:"caption"`
		Upload  File   `json:"upload"`
	}
	if err := Decode(data, &got); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if got.Caption != "a picture" {
		t.Errorf("Caption = %q", got.Caption)
	}
	if got.Upload.Name != "pic.bin" {
		t.Errorf("Upload.Name = %q, want pic.bin", got.Upload.Name)
	}
	if want := []byte{1, 2, 3, 4, 5}; string(got.Upload.Data) != string(want) {
		t.Errorf("Upload.Data = %v, want %v", got.Upload.Data, want)
	}
	if got.Upload.Size() != 5 {
		t.Errorf("Upload.Size() = %d, want 5", got.Upload.Size())
	}
}

func TestDecodeTwoFilesLandOnTheirOwnFields(t *testing.T) {
	data, _, err := Parse(decodeGolden(t, goldenTwoFiles))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	var got struct {
		Big   File `json:"big"`
		Small File `json:"small"`
	}
	if err := Decode(data, &got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if want := strings.Repeat("B", 20); string(got.Big.Data) != want {
		t.Errorf("Big.Data = %q, want %q", got.Big.Data, want)
	}
	if string(got.Small.Data) != "ss" {
		t.Errorf("Small.Data = %q, want %q", got.Small.Data, "ss")
	}
}

// A browser omits an unchecked checkbox and an empty number input entirely, so
// a field the submission did not carry has to be the zero value rather than an
// error.
func TestDecodeLeavesAbsentFieldsAlone(t *testing.T) {
	data, _, err := Parse(decodeGolden(t, goldenPlain))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	got := struct {
		Title     string `json:"title"`
		Subscribe bool   `json:"subscribe"`
		Quantity  int    `json:"quantity"`
	}{Subscribe: true, Quantity: 7}

	if err := Decode(data, &got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Title != "Hello" {
		t.Errorf("Title = %q", got.Title)
	}
	if !got.Subscribe || got.Quantity != 7 {
		t.Errorf("absent fields were overwritten: %+v", got)
	}
}

func TestDecodeRejectsMismatchedTypes(t *testing.T) {
	data, _, err := Parse(decodeGolden(t, goldenOneFile))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// `upload` is a File; a string field cannot hold one, and pretending it
	// can would silently drop the upload.
	var got struct {
		Upload string `json:"upload"`
	}
	err = Decode(data, &got)
	if err == nil {
		t.Fatal("Decode accepted a File into a string field")
	}
	if !errors.Is(err, ErrBadRequest) {
		t.Fatalf("error %v is not an ErrBadRequest", err)
	}
	if !strings.Contains(err.Error(), "upload") {
		t.Errorf("error %q does not name the offending field", err)
	}
}

func TestDecodeRequiresPointerTarget(t *testing.T) {
	var notAPointer struct{}
	if err := Decode(nil, notAPointer); err == nil {
		t.Fatal("Decode accepted a non-pointer target")
	}
}
