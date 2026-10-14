package dev

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// A request must not reread a project's source bytes just to serve a route.
func BenchmarkDevRequestWithSourceTree(b *testing.B) { benchmarkDevRequests(b, false) }

func BenchmarkDevConcurrentRequestsWithSourceTree(b *testing.B) { benchmarkDevRequests(b, true) }

func benchmarkDevRequests(b *testing.B, parallel bool) {
	root := b.TempDir()
	content := append([]byte("package fixture\n//"), bytes.Repeat([]byte("x"), 8192)...)
	for i := 0; i < 768; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("file%04d.go", i)), content, 0644); err != nil {
			b.Fatal(err)
		}
	}
	s, err := New(Config{Root: root, Web: filepath.Join(root, "web"), Args: func(string) []string { return nil }})
	if err != nil {
		b.Fatal(err)
	}
	s.built, err = s.fingerprint()
	if err != nil {
		b.Fatal(err)
	}
	s.settle(&generation{proxy: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("current fixture response")) })})
	b.ReportAllocs()
	b.ResetTimer()
	request := func() {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != 200 || w.Body.String() != "current fixture response" {
			b.Error(w.Code, w.Body.String())
		}
	}
	if parallel {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				request()
			}
		})
	} else {
		for b.Loop() {
			request()
		}
	}
}
