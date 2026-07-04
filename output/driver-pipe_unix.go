//go:build !windows

package output

import (
	"fmt"
	"os"
	"syscall"
)

// openPipeForWriting opens a FIFO non-blocking first to error out when no
// reader is attached, then restores blocking mode.
func openPipeForWriting(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open fifo: %w", err)
	}
	if err := syscall.SetNonblock(int(f.Fd()), false); err != nil {
		return nil, fmt.Errorf("failed to set blocking mode on fifo: %w", err)
	}
	return f, nil
}
