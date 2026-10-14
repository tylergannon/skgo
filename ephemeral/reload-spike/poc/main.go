package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	if err := serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func serve() error {
	source := flag.String("source", "handler.go.txt", "compatible Go source to watch")
	addr := flag.String("addr", "127.0.0.1:8097", "HTTP address")
	flag.Parse()
	dir, err := os.MkdirTemp("", "native-reload-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	r, err := newReloader(dir)
	if err != nil {
		return err
	}
	defer r.Close()
	body, err := os.ReadFile(*source)
	if err != nil {
		return err
	}
	if _, err := r.Reload(body); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: r, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(listener)
	defer server.Close()
	abs, _ := filepath.Abs(*source)
	fmt.Printf("pid=%d url=http://%s source=%s\n", os.Getpid(), listener.Addr(), abs)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	last := sha256.Sum256(body)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return server.Shutdown(context.Background())
		case <-ticker.C:
			body, err := os.ReadFile(*source)
			if err != nil {
				continue
			}
			sum := sha256.Sum256(body)
			if sum == last {
				continue
			}
			last = sum
			result, err := r.Reload(body)
			if err != nil {
				if errors.Is(err, ErrLoaderState) {
					return err
				}
				fmt.Fprintf(os.Stderr, "edit rejected; previous revision serves: %v\n", err)
				continue
			}
			_ = json.NewEncoder(os.Stdout).Encode(result)
		}
	}
}
