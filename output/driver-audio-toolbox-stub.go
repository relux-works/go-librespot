//go:build !darwin || !cgo

package output

import (
	"fmt"
)

func newAudioToolboxOutput(opts *NewOutputOptions) (Output, error) {
	return nil, fmt.Errorf("audio toolbox is only supported on MacOS")
}
