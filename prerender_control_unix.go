//go:build !windows

package skgo

import (
	"io"
	"net"
	"os"
	"syscall"
)

func prerenderControl() (io.ReadWriteCloser, error) {
	syscall.CloseOnExec(3)
	file := os.NewFile(3, "prerender-control")
	connection, err := net.FileConn(file)
	file.Close()
	return connection, err
}

func prerenderSignals() []os.Signal { return []os.Signal{os.Interrupt, syscall.SIGTERM} }
