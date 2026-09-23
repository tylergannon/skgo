package skgo

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"testing"
)

type captureFormTransport struct{ body []byte }

func (c *captureFormTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	var err error
	c.body, err = io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("captured form")
}

// From pinned Kit's serialize_binary_form({title:'Hello', body:'World'},
// {remote_refreshes:[]}); see internal/formdata/formdata_test.go.
const kitPlainForm = "AEYAAAAAAFtbMSw0XSx7InRpdGxlIjoyLCJib2R5IjozfSwiSGVsbG8iLCJXb3JsZCIseyJyZW1vdGVfcmVmcmVzaGVzIjo1fSxbXV0="

func TestFormClientUsesKitProducedEnvelope(t *testing.T) {
	transport := &captureFormTransport{}
	_, err := SubmitForm(context.Background(), FormClient{BaseURL: "http://fixture", HTTPClient: &http.Client{Transport: transport}}, "fixture/submit",
		struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}{Title: "Hello", Body: "World"},
		func(any) (string, error) { return "", nil })
	if err == nil {
		t.Fatal("capture did not stop request")
	}
	want, err := base64.StdEncoding.DecodeString(kitPlainForm)
	if err != nil {
		t.Fatal(err)
	}
	if string(transport.body) != string(want) {
		t.Fatalf("Form envelope differs from Kit fixture:\n got %x\nwant %x", transport.body, want)
	}
}
