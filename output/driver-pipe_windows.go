//go:build windows

package output

import (
	"fmt"
	"os"
	"strings"
)

// openPipeForWriting connects to a named pipe (\\.\pipe\...) created by
// the reader side (the Pulsar shell). Connecting fails when no server has
// the pipe open, which gives the same fail-fast semantics as the unix
// non-blocking FIFO probe. Same-package pipes work inside AppContainer
// under the \\.\pipe\LOCAL\ prefix.
func openPipeForWriting(path string) (*os.File, error) {
	if !strings.HasPrefix(path, `\\.\pipe\`) {
		return nil, fmt.Errorf("on windows pipe path must be \\\\.\\pipe\\<name>, got %q", path)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open named pipe (is the reader running?): %w", err)
	}
	return f, nil
}
