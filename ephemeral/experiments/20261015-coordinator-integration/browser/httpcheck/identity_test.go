//go:build skgo_listener_httpcheck

package httpcheck

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestActiveInstanceIdentityAndValue(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	for _, want := range []struct {
		name, url, publicOrigin, label, value string
	}{
		{"ALPHA", os.Getenv("ALPHA_URL"), "http://127.0.0.1:56612", "ALPHA", "Go SSR total: 4101"},
		{"BETA", os.Getenv("BETA_URL"), "http://127.0.0.1:56621", "BETA", "Go SSR total: 5202"},
	} {
		t.Run(want.name, func(t *testing.T) {
			if want.url == "" {
				t.Fatal("required URL environment variable is empty")
			}
			res, err := client.Get(strings.TrimRight(want.url, "/") + "/orders")
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}
			if res.StatusCode != http.StatusOK {
				t.Fatalf("status=%d body=%s", res.StatusCode, body)
			}
			if got := res.Header.Get("X-Application-Origin"); got != want.publicOrigin {
				t.Errorf("X-Application-Origin=%q, want %q", got, want.publicOrigin)
			}
			for _, literal := range []string{want.label, want.value} {
				if !strings.Contains(string(body), literal) {
					t.Errorf("response body lacks independent literal %q", literal)
				}
			}
		})
	}
}
