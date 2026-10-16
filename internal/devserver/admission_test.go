package devserver

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRequestAdmissionDrainsAndWaitsBeforeDispatch(t *testing.T) {
	var requests requestAdmission
	old, finish, err := requests.enter(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	drained := make(chan error, 1)
	go func() { drained <- requests.pause(context.Background(), time.Second) }()
	waitForPause(t, &requests)
	entered := make(chan struct{})
	go func() {
		_, release, err := requests.enter(context.Background(), time.Second)
		if err == nil {
			release()
			close(entered)
		}
	}()
	select {
	case <-drained:
		t.Fatal("retired an admitted request before it finished")
	case <-entered:
		t.Fatal("admitted a new request during handoff")
	case <-time.After(20 * time.Millisecond):
	}
	if old.Err() != nil {
		t.Fatal("canceled a finite request before the drain deadline")
	}
	finish()
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
		t.Fatal("admitted a request before replacement readiness")
	default:
	}
	requests.resume()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("waiting request did not resume")
	}
}

func TestRequestAdmissionBoundsStreamsWaitingAndShutdown(t *testing.T) {
	var requests requestAdmission
	stream, finish, err := requests.enter(context.Background(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	if err := requests.pause(context.Background(), 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(stream.Err(), context.Canceled) {
		t.Fatal("long stream survived the drain deadline")
	}
	if _, _, err := requests.enter(context.Background(), 10*time.Millisecond); !errors.Is(err, errRequestUnavailable) {
		t.Fatalf("waiting timeout = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	requests.resume()
	if _, _, err := requests.enter(canceled, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled request admitted after readiness: %v", err)
	}
	if err := requests.pause(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	waiting := make(chan error, 1)
	go func() {
		_, _, err := requests.enter(context.Background(), time.Second)
		waiting <- err
	}()
	requests.close()
	if err := <-waiting; !errors.Is(err, errRequestUnavailable) {
		t.Fatalf("shutdown waiter = %v", err)
	}
}

func waitForPause(t *testing.T, requests *requestAdmission) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		requests.mu.Lock()
		paused := requests.ready != nil
		requests.mu.Unlock()
		if paused {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("request admission did not pause")
}
