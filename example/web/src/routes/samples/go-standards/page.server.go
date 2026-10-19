package gostandards

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/tylergannon/skgo"
)

// Upload is what the upload action answers.
type Upload struct {
	Filename string   `json:"filename"`
	Bytes    int64    `json:"bytes"`
	SHA256   string   `json:"sha256"`
	Tags     []string `json:"tags"`
}

// upload reads the posted multipart form with the standard library, from the
// request that carried it.
func upload(ctx context.Context) (Upload, error) {
	r := skgo.EventFrom(ctx).Request()
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return Upload{}, skgo.Errorf(http.StatusBadRequest, "Invalid multipart form")
	}
	file, header, err := r.FormFile("upload")
	if err != nil {
		return Upload{}, skgo.Errorf(http.StatusBadRequest, "Choose a file")
	}
	defer file.Close()
	hash := sha256.New()
	count, err := io.Copy(hash, file)
	if err != nil {
		return Upload{}, err
	}
	return Upload{
		Filename: header.Filename,
		Bytes:    count,
		SHA256:   hex.EncodeToString(hash.Sum(nil)),
		Tags:     r.PostForm["tag"],
	}, nil
}

var _ = skgo.Action(upload)
