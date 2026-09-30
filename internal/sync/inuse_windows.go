package sync

import (
	"errors"
	"syscall"
)

// Windows reports a file open in another process as a sharing or lock
// violation rather than a permission error.
func isSharingViolation(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}

	const (
		errorSharingViolation syscall.Errno = 32
		errorLockViolation    syscall.Errno = 33
	)

	return errno == errorSharingViolation || errno == errorLockViolation
}
