//go:build !windows

package install

import (
	"errors"
	"syscall"
)

func isCrossVolume(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

// Detect finds nothing outside Windows; the player picks the folder.
func Detect() string {
	return ""
}
