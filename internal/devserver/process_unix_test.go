//go:build !windows

package devserver

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSweepRemovesOnlyAbandonedInstances(t *testing.T) {
	parent := t.TempDir()
	instance := func(name string) string {
		dir := filepath.Join(parent, name)
		if e := os.Mkdir(dir, 0o700); e != nil {
			t.Fatal(e)
		}
		return dir
	}
	live, abandoned, unclaimed := instance("skgo-live"), instance("skgo-abandoned"), instance("skgo-unclaimed")
	claim, e := claimInstance(live)
	if e != nil {
		t.Fatal(e)
	}
	defer claim.Close()
	ended, e := claimInstance(abandoned)
	if e != nil {
		t.Fatal(e)
	}
	ended.Close()

	sweepInstances(parent)
	for dir, kept := range map[string]bool{live: true, abandoned: false, unclaimed: true} {
		if _, e := os.Stat(dir); (e == nil) != kept {
			t.Errorf("%s: kept=%v, want %v (%v)", filepath.Base(dir), e == nil, kept, e)
		}
	}
}

func TestChildInputEndsOnlyWithSession(t *testing.T) {
	read, write, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer read.Close()
	c, e := startChild(context.Background(), t.TempDir(), "sh", []string{"-c", "cat; echo end"}, os.Environ(), write)
	write.Close()
	if e != nil {
		t.Fatal(e)
	}
	defer c.close()
	_ = read.SetReadDeadline(time.Now().Add(10 * time.Second))
	lines := bufio.NewScanner(read)
	next := func() string {
		if !lines.Scan() {
			t.Fatalf("child output ended: %v", lines.Err())
		}
		return lines.Text()
	}
	if _, e = c.lifeline.WriteString("held\n"); e != nil {
		t.Fatal(e)
	}
	if got := next(); got != "held" {
		t.Fatalf("child read %q from its input, want the session's write", got)
	}
	c.lifeline.Close()
	if got := next(); got != "end" {
		t.Fatalf("child printed %q after the session released its input, want end", got)
	}
	<-c.done
	if c.err != nil {
		t.Fatalf("child exited with %v", c.err)
	}
}
