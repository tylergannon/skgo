//go:build windows

package skgo

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// Node supplies a duplex named pipe as this helper's dedicated standard input.
// os.Stdin wraps the inherited Windows handle, rather than a Unix descriptor.
func prerenderControl() (io.ReadWriteCloser, error) {
	if err := windows.SetHandleInformation(windows.Handle(os.Stdin.Fd()), windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return nil, err
	}
	return os.Stdin, nil
}

func prerenderSignals() []os.Signal { return []os.Signal{os.Interrupt} }
