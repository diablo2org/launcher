//go:build !windows

package app

func osVersion() string {
	return ""
}
