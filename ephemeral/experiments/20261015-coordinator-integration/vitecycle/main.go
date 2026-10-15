package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	root, _ := filepath.Abs("ephemeral/experiments/20261015-coordinator-integration/runtime/ALPHA")
	res, e := http.Get("http://127.0.0.1:56613/__tool/identity")
	if e != nil {
		panic(e)
	}
	var before struct {
		PID  int
		Root string
	}
	e = json.NewDecoder(res.Body).Decode(&before)
	res.Body.Close()
	if e != nil || before.Root != root {
		panic(fmt.Sprint("not owned root", before, e))
	}
	group, e := syscall.Getpgid(before.PID)
	if e != nil {
		panic(e)
	}
	fmt.Printf("STOP_VITE ownRoot=%s pid=%d group=%d\n", root, before.PID, group)
	if e = syscall.Kill(-group, syscall.SIGKILL); e != nil {
		panic(e)
	}
	fmt.Println("VITE_DOWN waiting own VITE_RESTART marker")
	for {
		if _, e := os.Stat(filepath.Join(root, "VITE_RESTART")); e == nil {
			os.Remove(filepath.Join(root, "VITE_RESTART"))
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	cmd := exec.Command("vp", "dev")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PRIVATE_VITE_PORT=56613", "PUBLIC_PORT=56612", "PUBLIC_ORIGIN=http://127.0.0.1:56612", "INSTANCE=ALPHA")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	log, e := os.Create("ephemeral/experiments/20261015-coordinator-integration/vite-restarted.log")
	if e != nil {
		panic(e)
	}
	defer log.Close()
	cmd.Stdout = log
	cmd.Stderr = log
	if e = cmd.Start(); e != nil {
		panic(e)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait(); close(done) }()
	defer func() { syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); <-done }()
	fmt.Printf("RESTART_VITE launcher=%d publicOriginUnchanged=http://127.0.0.1:56612\n", cmd.Process.Pid)
	for {
		select {
		case e := <-done:
			panic(fmt.Sprint("new Vite exited ", e))
		case <-time.After(100 * time.Millisecond):
			if _, e := os.Stat(filepath.Join(root, "VITE_STOP")); e == nil {
				fmt.Println(strings.ToUpper("owned restarted vite stop"))
				return
			}
		}
	}
}
