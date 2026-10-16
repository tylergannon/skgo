//go:build !windows

package devserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const ownedGeneratorFixture = `package main
import("os";"os/exec";"strconv";"time")
func main(){
 if len(os.Args)>1 {for{time.Sleep(time.Hour)}}
 cmd:=exec.Command(os.Args[0],"descendant");cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr
 if err:=cmd.Start();err!=nil{panic(err)}
 if err:=os.WriteFile("descendant.pid",[]byte(strconv.Itoa(cmd.Process.Pid)),0644);err!=nil{panic(err)}
 for{time.Sleep(time.Hour)}
}
`

func TestGeneratorReplacementAndCanceledPrerequisiteOwnDescendants(t *testing.T) {
	g, root, log := generatorFixture(t)
	source := filepath.Join(root, "tool/main.go")
	if err := os.WriteFile(source, []byte(ownedGeneratorFixture), 0644); err != nil {
		t.Fatal(err)
	}
	readPID := func(previous int) int {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			data, err := os.ReadFile(filepath.Join(root, "descendant.pid"))
			if err == nil {
				pid, _ := strconv.Atoi(string(data))
				if pid != 0 && pid != previous {
					return pid
				}
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("descendant not started\n%s", log)
		return 0
	}
	dead := func(pid int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("owned descendant %d survived", pid)
	}
	if _, err := g.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := readPID(0)
	if err := os.WriteFile(source, []byte(ownedGeneratorFixture+"\n// fresh helper identity\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := g.refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	second := readPID(first)
	dead(first)
	g.close()
	dead(second)
	if err := os.Remove(filepath.Join(root, "descendant.pid")); err != nil {
		t.Fatal(err)
	}
	cfgJSON := fmt.Sprintf(`{"package":"./tool","beforeBuild":{"argv":[%q]}}`, filepath.Join(g.work, "generator"))
	if err := os.WriteFile(g.config, []byte(cfgJSON), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := g.readConfig()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- g.prerequisite(ctx, cfg) }()
	third := readPID(0)
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "beforeBuild") {
			t.Fatalf("cancel result=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("prerequisite cancellation did not join")
	}
	dead(third)
}
