//go:build skgo_dev_integration

package devserver

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRealSessionNativeRelocationRecovery(t *testing.T) {
	realSession(t, false, false, "relocation")
}

func mutexSessionRoute(value string, initial int) string {
	source := sessionRoute(value, initial)
	source = strings.Replace(source, `import("fmt";`, `import("sync";"fmt";`, 1)
	source = strings.Replace(source, `var initial=`, "var mu sync.Mutex\nvar initial=", 1)
	return strings.Replace(source, `func get(w http.ResponseWriter,r *http.Request){`, `func get(w http.ResponseWriter,r *http.Request){mu.Lock();defer mu.Unlock();`, 1)
}

func exerciseSessionRelocation(t *testing.T, app, origin string, log *sessionLog) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	request := func(want, pid string) {
		t.Helper()
		res, err := client.Get(origin + "/orders")
		if err != nil {
			t.Fatalf("native relocation interrupted application: %v\n%s", err, log.String())
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 || string(body) != want || res.Header.Get("X-Go-Pid") != pid {
			t.Fatalf("application status=%d body=%s pid=%s, want 200 %s pid=%s: %v\n%s", res.StatusCode, body, res.Header.Get("X-Go-Pid"), want, pid, err, log.String())
		}
	}
	pid := strconv.Itoa(log.lastPID())
	if pid == "0" {
		t.Fatal("missing actual native process identity")
	}
	request("literal-4101:8", pid)
	logStart := len(log.String())
	route := filepath.Join(app, "web/src/routes/orders/server.go")
	if err := os.WriteFile(route, []byte(mutexSessionRoute("5202", 8)), 0644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(25 * time.Second)
	for !strings.Contains(log.String()[logStart:], "rebuilding revision") && time.Now().Before(deadline) {
		request("literal-4101:8", pid)
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(log.String()[logStart:], "rebuilding revision") {
		t.Fatalf("native relocation failed without bounded rebuild diagnostic\n%s", log.String())
	}
	if output := log.String()[logStart:]; !strings.Contains(output, "after install admission") || !strings.Contains(output, "symName:sync.(*Mutex).Unlock relocateType:R_ADDROFF") {
		t.Fatalf("fixture did not reproduce the mutex relocation failure\n%s", output)
	}
	wait := func(want string) string {
		t.Helper()
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			res, err := client.Get(origin + "/orders")
			if err == nil {
				body, _ := io.ReadAll(res.Body)
				res.Body.Close()
				if res.StatusCode == 200 && string(body) == want {
					return res.Header.Get("X-Go-Pid")
				}
				if want == "literal-5202:8" && (res.StatusCode != 200 || string(body) != "literal-4101:8" || res.Header.Get("X-Go-Pid") != pid) {
					t.Fatalf("fallback lost healthy application before replacement: status=%d body=%s pid=%s\n%s", res.StatusCode, body, res.Header.Get("X-Go-Pid"), log.String())
				}
			} else if want == "literal-5202:8" {
				t.Fatalf("fallback interrupted healthy application: %v\n%s", err, log.String())
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("edit never became current: %s\n%s", want, log.String())
		return ""
	}
	replacement := wait("literal-5202:8")
	if replacement == "" || replacement == pid {
		t.Fatalf("fallback did not replace native PID %s -> %s\n%s", pid, replacement, log.String())
	}
	if err := os.WriteFile(route, []byte(sessionRoute("6303", 8)), 0644); err != nil {
		t.Fatal(err)
	}
	corrected := wait("literal-6303:8")
	if corrected == "" || corrected == replacement {
		t.Fatalf("correcting mutex declaration did not replace native PID %s -> %s\n%s", replacement, corrected, log.String())
	}
	if err := os.WriteFile(route, []byte(sessionRoute("7407", 8)), 0644); err != nil {
		t.Fatal(err)
	}
	if native := wait("literal-7407:8"); native != corrected {
		t.Fatalf("corrected session lost native edit path: %s -> %s\n%s", corrected, native, log.String())
	}
	correctedPID, err := strconv.Atoi(corrected)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(correctedPID, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	if recovered := wait("literal-7407:8"); recovered == "" || recovered == corrected {
		t.Fatalf("child recovery failed after relocation and corrected edit: %s -> %s\n%s", corrected, recovered, log.String())
	}
	t.Logf("relocation fallback PID %s -> %s; corrected PID %s retained through native edit\n%s", pid, replacement, corrected, log.String())
}
