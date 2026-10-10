package app

import "testing"

func TestDevEnv(t *testing.T) {
	t.Setenv("LAUNCHER_TEST_SETTING", "https://127.0.0.1:8667")
	defer func(v string) { Version = v }(Version)

	Version = "dev"
	if got := DevEnv("LAUNCHER_TEST_SETTING"); got != "https://127.0.0.1:8667" {
		t.Errorf("development build: %q", got)
	}

	Version = "v1.2.3"
	if got := DevEnv("LAUNCHER_TEST_SETTING"); got != "" {
		t.Errorf("release build: %q, want it ignored", got)
	}
}
