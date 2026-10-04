//go:build !unix

package dev

import "os/exec"

func ownGroup(*exec.Cmd) {}

func signalGroup(cmd *exec.Cmd, _ bool) { _ = cmd.Process.Kill() }

func processGroupExists(*exec.Cmd) bool { return false }
