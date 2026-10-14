package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWatchedFileHost(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "native-reload-poc")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build host: %v: %s", err, out)
	}
	source := fixture(t)
	path := filepath.Join(dir, "handler.go")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-source", path, "-addr", "127.0.0.1:0")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	events := make(chan string, 100)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			events <- scanner.Text()
		}
		close(events)
	}()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
		}
	}()
	waitEvent := func(fragment string) string {
		t.Helper()
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		for {
			select {
			case line, ok := <-events:
				if !ok {
					t.Fatal("host exited before expected event")
				}
				t.Log(line)
				if strings.Contains(line, fragment) {
					return line
				}
			case <-timer.C:
				t.Fatalf("missing host event %q", fragment)
			}
		}
	}
	line := waitEvent("url=http://")
	parts := regexp.MustCompile(`pid=(\d+) url=(\S+) source=`).FindStringSubmatch(line)
	if len(parts) != 3 {
		t.Fatalf("invalid startup line %q", line)
	}
	pid, url := parts[1], parts[2]
	if pid != strconv.Itoa(cmd.Process.Pid) {
		t.Fatal("wrong serving PID")
	}
	client := &http.Client{Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	request := func(route string) string {
		t.Helper()
		resp, err := client.Get(url + route)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.Header.Get("X-Host-PID") != pid {
			t.Fatal("host restarted")
		}
		return string(body)
	}
	expect := func(want string) {
		t.Helper()
		if got := request("/proof?n=7"); got != want {
			t.Fatalf("HTTP %q; want %q", got, want)
		}
	}
	edit := func(body []byte, wantLiteral string) {
		t.Helper()
		start := time.Now()
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			if got := request("/literal"); got == wantLiteral {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("watched edit never became active")
			}
			time.Sleep(time.Millisecond)
		}
		t.Logf("watched source-write to HTTP pid=%s literal=%q elapsed_ms=%.3f", pid, wantLiteral, float64(time.Since(start))/float64(time.Millisecond))
	}
	expect("original:1:21:/proof")
	edit(edited(source, "watched", 5), "watched-literal")
	expect("watched:2:35:/proof")
	broken := strings.ReplaceAll(string(edited(source, "broken", 99)), "n * 99", "missingIdentifier")
	if err := os.WriteFile(path, []byte(broken), 0600); err != nil {
		t.Fatal(err)
	}
	waitEvent("edit rejected")
	expect("watched:3:35:/proof")
	edit(edited(source, "repaired", 7), "repaired-literal")
	expect("repaired:4:49:/proof")
	edit(source, "original-literal")
	expect("original:5:21:/proof")
	incompatible := strings.Replace(string(source), "pause func()) string", "pause func()) int", 1)
	if err := os.WriteFile(path, []byte(incompatible), 0600); err != nil {
		t.Fatal(err)
	}
	waitEvent("normal rebuild/restart required")
	expect("original:6:21:/proof")
	t.Log(fmt.Sprintf("actual watched-file host pid=%s retained across edit, compiler error, repair, source revert and incompatible signature rejection", pid))
}
