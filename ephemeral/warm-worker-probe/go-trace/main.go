// A profiling-only Go command proxy. It preserves stdout/stderr and exit status.
package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"time"
)

func main() {
	start := time.Now()
	cmd := exec.Command(os.Getenv("SKGO_REAL_GO"), os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if path := os.Getenv("SKGO_GO_TRACE"); path != "" {
		f, e := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if e == nil {
			_ = json.NewEncoder(f).Encode(map[string]any{"args": os.Args[1:], "milliseconds": float64(time.Since(start).Microseconds()) / 1000})
			_ = f.Close()
		}
	}
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			os.Exit(e.ExitCode())
		}
		os.Exit(1)
	}
}
