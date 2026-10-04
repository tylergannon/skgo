//go:build unix

package dev

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const processFixtureEnv = "SKGO_DEV_PROCESS_FIXTURE_"

type processFixtureInfo struct {
	PID     int    `json:"pid"`
	Address string `json:"address"`
}

// TestOwnedProcessFixture is a subprocess fixture. Its launcher exits while a
// same-group worker can keep a listener open, optionally ignoring SIGTERM.
func TestOwnedProcessFixture(t *testing.T) {
	role := os.Getenv(processFixtureEnv + "ROLE")
	if role == "" {
		return
	}
	if role == "worker" {
		if os.Getenv(processFixtureEnv+"IGNORE_TERM") == "1" {
			signal.Ignore(syscall.SIGTERM)
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(121)
		}
		info := processFixtureInfo{PID: os.Getpid(), Address: listener.Addr().String()}
		data, err := json.Marshal(info)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(122)
		}
		if err := os.WriteFile(os.Getenv(processFixtureEnv+"READY"), data, 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(123)
		}
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}

	if role != "leader-exit" && role != "leader-wait" {
		fmt.Fprintf(os.Stderr, "unknown fixture role %q\n", role)
		os.Exit(124)
	}
	worker := exec.Command(os.Args[0], "-test.run=^TestOwnedProcessFixture$")
	worker.Env = fixtureEnv(os.Environ(), map[string]string{
		"ROLE":        "worker",
		"READY":       os.Getenv(processFixtureEnv + "READY"),
		"IGNORE_TERM": os.Getenv(processFixtureEnv + "IGNORE_TERM"),
	})
	if os.Getenv(processFixtureEnv+"INHERIT_IO") != "1" {
		worker.Stdout = nil
		worker.Stderr = nil
	} else {
		worker.Stdout = os.Stdout
		worker.Stderr = os.Stderr
	}
	if err := worker.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(125)
	}
	if _, err := awaitFixtureInfo(os.Getenv(processFixtureEnv+"READY"), 5*time.Second); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(126)
	}

	if role == "leader-exit" {
		fmt.Fprintln(os.Stdout, "leader-output-marker")
		os.Exit(7)
	}
	select {}
}

func TestProcessStopKillsDescendantAfterLeaderExit(t *testing.T) {
	p, info := startProcessFixture(t, "leader-exit", false, false, 0)
	awaitDone(t, p, 5*time.Second)
	group := p.PID()
	assertFixtureGroup(t, group, info.PID)
	if err := p.ExitError(); err == nil || err.Error() != "exit status 7" {
		t.Fatalf("leader exit error = %v, want exit status 7", err)
	}
	beforeState, beforeGroup, beforeListener := processObservations(info, group)
	if !stateIsLive(beforeState) || !beforeListener {
		t.Fatalf("fixture did not leave a live listener before Stop: pid=%d state=%q group=%s listenerOpen=%t", info.PID, beforeState, beforeGroup, beforeListener)
	}
	t.Logf("before Stop: leader=%d exit=%v descendant=%d pgid=%d state=%s listener=%s open=%t group=%s", p.PID(), p.ExitError(), info.PID, group, beforeState, info.Address, beforeListener, beforeGroup)

	started := time.Now()
	p.Stop(500 * time.Millisecond)
	p.Stop(time.Second) // a completed Stop remains safe and idempotent.
	afterState, afterGroup, afterListener := processObservations(info, group)
	t.Logf("after Stop: leader=%d exit=%v descendant=%d pgid=%d state=%s listener=%s open=%t group=%s elapsed=%s", p.PID(), p.ExitError(), info.PID, group, afterState, info.Address, afterListener, afterGroup, time.Since(started))
	assertFixtureStopped(t, info, group, afterState, afterGroup, afterListener)
}

func TestProcessStopEscalatesAfterLeaderExitsDuringStop(t *testing.T) {
	p, info := startProcessFixture(t, "leader-wait", true, false, 0)
	group := p.PID()
	assertFixtureGroup(t, group, info.PID)
	beforeState, beforeGroup, beforeListener := processObservations(info, group)
	if !stateIsLive(beforeState) || !beforeListener {
		t.Fatalf("fixture did not start live: pid=%d state=%q group=%s listenerOpen=%t", info.PID, beforeState, beforeGroup, beforeListener)
	}
	t.Logf("before Stop: leader=%d descendant=%d pgid=%d state=%s listener=%s open=%t group=%s", p.PID(), info.PID, group, beforeState, info.Address, beforeListener, beforeGroup)

	const grace = 800 * time.Millisecond
	started := time.Now()
	var wg sync.WaitGroup
	stopReturned := make(chan time.Time, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.Stop(grace)
			stopReturned <- time.Now()
		}()
	}
	awaitDone(t, p, 2*time.Second)
	leaderExit := p.ExitError()
	var processExit *exec.ExitError
	if !errors.As(leaderExit, &processExit) || processExit.ExitCode() == 0 {
		t.Fatalf("leader ExitError = %v after its Done channel closed, want nonzero process ExitError; output=%q", leaderExit, p.tail.String())
	}
	stateDuringGrace, groupDuringGrace, listenerDuringGrace := processObservations(info, group)
	if !stateIsLive(stateDuringGrace) || !listenerDuringGrace {
		t.Fatalf("TERM-ignoring descendant did not survive for escalation proof: state=%q group=%s listenerOpen=%t", stateDuringGrace, groupDuringGrace, listenerDuringGrace)
	}
	noEarlyReturn := time.Until(started.Add(grace * 3 / 4))
	select {
	case <-stopReturned:
		t.Fatalf("Stop returned after leader exit while owned descendant remained live: state=%q group=%s", stateDuringGrace, groupDuringGrace)
	case <-time.After(noEarlyReturn):
	}
	t.Logf("leader exited during grace: leader=%d exit=%v descendant=%d state=%s listener=%s open=%t group=%s", p.PID(), leaderExit, info.PID, stateDuringGrace, info.Address, listenerDuringGrace, groupDuringGrace)

	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	var lastReturn time.Time
	for range 4 {
		select {
		case returned := <-stopReturned:
			if returned.After(lastReturn) {
				lastReturn = returned
			}
		case <-deadline.C:
			t.Fatal("concurrent Stop calls did not all return")
		}
	}
	wg.Wait()
	elapsed := lastReturn.Sub(started)
	if elapsed < grace-50*time.Millisecond {
		t.Fatalf("Stop elapsed %s, want it to wait through %s grace for the live descendant", elapsed, grace)
	}
	afterState, afterGroup, afterListener := processObservations(info, group)
	t.Logf("after Stop escalation: leader=%d exit=%v descendant=%d pgid=%d state=%s listener=%s open=%t group=%s elapsed=%s", p.PID(), leaderExit, info.PID, group, afterState, info.Address, afterListener, afterGroup, elapsed)
	if p.ExitError() != leaderExit {
		t.Fatalf("leader ExitError changed during Stop: before=%v after=%v", leaderExit, p.ExitError())
	}
	assertFixtureStopped(t, info, group, afterState, afterGroup, afterListener)
}

func TestStartProcessBoundsInheritedPipesAndPreservesWaitDelay(t *testing.T) {
	tests := []struct {
		name      string
		waitDelay time.Duration
		minimum   time.Duration
	}{
		{name: "default", minimum: 100 * time.Millisecond},
		{name: "caller-supplied", waitDelay: 600 * time.Millisecond, minimum: 500 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			started := time.Now()
			p, info := startProcessFixture(t, "leader-exit", true, true, tt.waitDelay)
			group := p.PID()
			assertFixtureGroup(t, group, info.PID)
			if p.cmd.WaitDelay < tt.minimum {
				t.Fatalf("effective WaitDelay = %s, want at least %s", p.cmd.WaitDelay, tt.minimum)
			}
			awaitDone(t, p, 3*time.Second)
			waitElapsed := time.Since(started)
			if waitElapsed < tt.minimum {
				t.Fatalf("Done closed after %s, want at least %s for the configured pipe wait", waitElapsed, tt.minimum)
			}
			if err := p.ExitError(); err == nil || err.Error() != "exit status 7" {
				t.Fatalf("leader exit error = %v, want exit status 7", err)
			}
			if !strings.Contains(p.tail.String(), "leader-output-marker") {
				t.Fatalf("leader output was lost across bounded pipe settlement: %q", p.tail.String())
			}
			state, groupState, listenerOpen := processObservations(info, group)
			if !stateIsLive(state) || !listenerOpen {
				t.Fatalf("inherited-pipe fixture did not retain its worker: state=%q group=%s listenerOpen=%t", state, groupState, listenerOpen)
			}
			t.Logf("Wait returned with descendant still owned: leader=%d exit=%v descendant=%d pgid=%d state=%s listener=%s open=%t group=%s waitDelay=%s elapsed=%s tail=%q", p.PID(), p.ExitError(), info.PID, group, state, info.Address, listenerOpen, groupState, p.cmd.WaitDelay, waitElapsed, p.tail.String())

			p.Stop(250 * time.Millisecond)
			state, groupState, listenerOpen = processObservations(info, group)
			t.Logf("after Stop: leader=%d exit=%v descendant=%d pgid=%d state=%s listener=%s open=%t group=%s", p.PID(), p.ExitError(), info.PID, group, state, info.Address, listenerOpen, groupState)
			assertFixtureStopped(t, info, group, state, groupState, listenerOpen)
		})
	}
}

func startProcessFixture(t *testing.T, role string, ignoreTerm, inheritIO bool, waitDelay time.Duration) (*Process, processFixtureInfo) {
	t.Helper()
	ready := filepath.Join(t.TempDir(), "worker.json")
	cmd := exec.Command(os.Args[0], "-test.run=^TestOwnedProcessFixture$")
	cmd.Env = fixtureEnv(os.Environ(), map[string]string{
		"ROLE":        role,
		"READY":       ready,
		"IGNORE_TERM": boolString(ignoreTerm),
		"INHERIT_IO":  boolString(inheritIO),
	})
	cmd.WaitDelay = waitDelay
	p, err := StartProcess(cmd, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if processGroupExists(p.cmd) {
			// This test created this exact Setpgid group; cleanup never targets
			// a process selected by name, port, or a machine-wide process list.
			signalGroup(p.cmd, true)
		}
		select {
		case <-p.Done():
		case <-time.After(3 * time.Second):
		}
		waitForProcessGroupExit(p.cmd, processKillWait)
	})
	info, err := awaitFixtureInfo(ready, 5*time.Second)
	if err != nil {
		t.Fatalf("%v; leader output: %q", err, p.tail.String())
	}
	return p, info
}

func fixtureEnv(base []string, values map[string]string) []string {
	filtered := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		if !strings.HasPrefix(entry, processFixtureEnv) {
			filtered = append(filtered, entry)
		}
	}
	for key, value := range values {
		filtered = append(filtered, processFixtureEnv+key+"="+value)
	}
	return filtered
}

func boolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func awaitFixtureInfo(path string, timeout time.Duration) (processFixtureInfo, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			var info processFixtureInfo
			if err := json.Unmarshal(data, &info); err != nil {
				return processFixtureInfo{}, err
			}
			return info, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return processFixtureInfo{}, fmt.Errorf("timed out waiting for listener fixture %q", path)
}

func awaitDone(t *testing.T, p *Process, timeout time.Duration) {
	t.Helper()
	select {
	case <-p.Done():
	case <-time.After(timeout):
		t.Fatalf("leader pid %d did not finish within %s", p.PID(), timeout)
	}
}

func assertFixtureGroup(t *testing.T, group, pid int) {
	t.Helper()
	actual, err := syscall.Getpgid(pid)
	if err != nil {
		t.Fatalf("worker %d has no process group: %v", pid, err)
	}
	if actual != group {
		t.Fatalf("worker %d has pgid %d, want the exact owned group %d", pid, actual, group)
	}
}

func processObservations(info processFixtureInfo, group int) (state, groupState string, listenerOpen bool) {
	state = processState(info.PID)
	groupState = processGroupSnapshot(group)
	conn, err := net.DialTimeout("tcp", info.Address, 75*time.Millisecond)
	if err == nil {
		listenerOpen = true
		_ = conn.Close()
	}
	return state, groupState, listenerOpen
}

func processState(pid int) string {
	output, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).CombinedOutput()
	state := strings.TrimSpace(string(output))
	if state == "" {
		killErr := syscall.Kill(pid, 0)
		if killErr == syscall.ESRCH {
			return "<gone>"
		}
		if err != nil {
			return fmt.Sprintf("<ps unavailable: %v; kill(pid, 0)=%v>", err, killErr)
		}
		return "<gone>"
	}
	return state
}

func processGroupSnapshot(group int) string {
	output, err := exec.Command("ps", "-Ao", "pid=,pgid=,stat=").CombinedOutput()
	var rows []string
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pgid, parseErr := strconv.Atoi(fields[1])
		if parseErr == nil && pgid == group {
			rows = append(rows, strings.Join(fields, " "))
		}
	}
	if len(rows) == 0 {
		groupErr := syscall.Kill(-group, 0)
		if groupErr == syscall.ESRCH {
			return "<empty>"
		}
		if err != nil {
			return fmt.Sprintf("<ps unavailable: %v; kill(group, 0)=%v>", err, groupErr)
		}
		return "<empty>"
	}
	return strings.Join(rows, "; ")
}

func stateIsLive(state string) bool {
	return state != "<gone>" && !strings.HasPrefix(state, "Z") && !strings.HasPrefix(state, "<zombie")
}

func assertFixtureStopped(t *testing.T, info processFixtureInfo, group int, state, groupState string, listenerOpen bool) {
	t.Helper()
	if listenerOpen {
		t.Fatalf("owned listener %s still accepts connections after Stop", info.Address)
	}
	if stateIsLive(state) {
		t.Fatalf("owned descendant %d remains live after Stop (state=%q pgid=%d group=%s)", info.PID, state, group, groupState)
	}
	if state == "<gone>" {
		return
	}
	if strings.HasPrefix(state, "<ps unavailable:") {
		t.Fatalf("could not inspect descendant state after Stop: %s", state)
	}
	if state == "" || state[0] != 'Z' {
		t.Fatalf("unexpected descendant state after Stop: %q", state)
	}
	t.Logf("descendant %d is terminated and awaiting OS reaping (state=%s); listener is closed", info.PID, state)
}
