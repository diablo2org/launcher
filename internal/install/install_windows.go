package install

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

const errorNotSameDevice syscall.Errno = 17

func isCrossVolume(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == errorNotSameDevice
}

// Detect returns the install the game itself last recorded, from the
// InstallPath registry value, if it still holds d2data.mpq.
func Detect() string {
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		k, err := registry.OpenKey(root, `Software\Blizzard Entertainment\Diablo II`, registry.QUERY_VALUE)
		if err != nil {
			continue
		}

		path, _, err := k.GetStringValue("InstallPath")
		k.Close()
		if err != nil || path == "" {
			continue
		}

		path = filepath.Clean(path)
		if _, err := os.Stat(filepath.Join(path, "d2data.mpq")); err == nil {
			return path
		}
	}

	return ""
}
