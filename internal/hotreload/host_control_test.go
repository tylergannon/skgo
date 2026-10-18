package hotreload

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func controlHost(t *testing.T) (*Host, *os.File, *os.File) {
	t.Helper()
	commandRead, commandWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	replyRead, replyWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		commandRead.Close()
		commandWrite.Close()
		replyRead.Close()
		replyWrite.Close()
	})
	return &Host{commands: commandWrite, replies: replyRead, installed: map[string][]byte{"route.go": []byte("literal-old")}}, commandRead, replyWrite
}

func TestNativeInstallationFailureRequiresReplacement(t *testing.T) {
	host, commands, replies := controlHost(t)
	child := make(chan error, 1)
	go func() {
		var command hostMessage
		if err := json.NewDecoder(commands).Decode(&command); err != nil {
			child <- err
			return
		}
		child <- json.NewEncoder(replies).Encode(hostReply{Error: "symName:sync.(*Mutex).Unlock relocateType:R_ADDROFF, symbol is not exist!"})
	}()
	err := host.Install(context.Background(), &HostUpdate{files: map[string][]byte{"route.go": []byte("literal-unapplied")}})
	var rebuild *CompleteBuild
	if !errors.As(err, &rebuild) || !strings.Contains(err.Error(), "sync.(*Mutex).Unlock") {
		t.Fatalf("installation error must preserve relocation diagnostic and request replacement: %v", err)
	}
	if got := string(host.installed["route.go"]); got != "literal-old" {
		t.Fatalf("failed installation recorded unapplied source: %s", got)
	}
	if err := <-child; err != nil {
		t.Fatal(err)
	}
}

func TestNativeInstallationTimeoutRejectsLateAcknowledgement(t *testing.T) {
	host, commands, replies := controlHost(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	// A blocked child never acknowledges this first command.
	started := time.Now()
	err := host.Install(ctx, &HostUpdate{files: map[string][]byte{"route.go": []byte("literal-unapplied")}})
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("installation did not honor bounded cancellation: %v", err)
	}
	var command hostMessage
	if err := json.NewDecoder(commands).Decode(&command); err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(replies).Encode(hostReply{}); err != nil {
		t.Fatal(err)
	}
	err = host.Install(context.Background(), &HostUpdate{files: map[string][]byte{"route.go": []byte("literal-later")}})
	var rebuild *CompleteBuild
	if !errors.As(err, &rebuild) {
		t.Fatalf("timed-out stream consumed a stale successful acknowledgement: %v", err)
	}
	if got := string(host.installed["route.go"]); got != "literal-old" {
		t.Fatalf("uncertain installation recorded unapplied source: %s", got)
	}
	if err := commands.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(commands).Decode(&command); err == nil {
		t.Fatal("sent another command on a stream with an unacknowledged installation")
	}
}

func TestNativeControlReadStopsOnSessionCancellation(t *testing.T) {
	host, commands, _ := controlHost(t)
	ctx, cancel := context.WithCancel(context.Background())
	child := make(chan struct{})
	go func() {
		var command hostMessage
		_ = json.NewDecoder(commands).Decode(&command)
		cancel()
		close(child)
	}()
	started := time.Now()
	err := host.Install(ctx, &HostUpdate{})
	if !errors.Is(err, context.Canceled) || time.Since(started) > time.Second {
		t.Fatalf("session cancellation left installation blocked: %v", err)
	}
	<-child
}
