//go:build !windows

package sync

func isSharingViolation(error) bool { return false }
