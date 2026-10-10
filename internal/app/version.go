package app

import (
	"log/slog"
	"os"
)

// Version is set at build time with -ldflags "-X github.com/diablo2org/launcher/internal/app.Version=v1.2.3".
var Version = "dev"

// Released reports whether this is a tagged release, not a development build.
func Released() bool {
	return Version != "dev"
}

// DevEnv returns a development setting from the environment, or "" in a
// release build. Each one points the launcher at other servers or another
// certificate, or opens its page to other programs: what a developer testing
// the launcher wants, and nothing a player should have. One set on a
// release is logged and ignored.
func DevEnv(name string) string {
	v := os.Getenv(name)
	if v != "" && Released() {
		slog.Warn("ignoring a development setting in a release build", "name", name)
		return ""
	}

	return v
}
