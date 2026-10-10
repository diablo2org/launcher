package pack

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diablo2org/launcher/internal/spec"
)

// keyFile writes a new key and returns it with its public half.
func keyFile(t *testing.T, dir string) (string, string) {
	t.Helper()

	private, public, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "signing-key.pem")
	if err := os.WriteFile(file, private, 0o600); err != nil {
		t.Fatal(err)
	}

	return file, public
}

// signProfile adds signing keys to the build test's profile.
func signProfile(t *testing.T, root string, keys ...string) {
	t.Helper()

	var doc map[string]interface{}
	json.Unmarshal([]byte(buildProfile), &doc)
	doc["signing"] = map[string]interface{}{"keys": keys}
	data, _ := json.Marshal(doc)
	write(t, root, "profile.json", string(data))
}

func TestBuildSigned(t *testing.T) {
	plan, root := buildSetup(t)
	file, public := keyFile(t, t.TempDir())
	key, err := LoadKey(file)
	if err != nil {
		t.Fatal(err)
	}
	if spec.FormatPublicKey(key.Public().(ed25519.PublicKey)) != public {
		t.Fatal("loaded key isn't the one generated")
	}

	// Unsigned profile: a key would sign for nothing.
	if _, err := Build(plan, BuildOptions{Version: "1", Out: filepath.Join(root, "a"), Key: key}); err == nil || !strings.Contains(err.Error(), public) {
		t.Errorf("key for an unsigned profile: %v", err)
	}

	signProfile(t, root, public)

	// Signed profile: no key, or a key it doesn't list, is refused.
	if _, err := Build(plan, BuildOptions{Version: "1", Out: filepath.Join(root, "b")}); err == nil || !strings.Contains(err.Error(), "-key") {
		t.Errorf("no key: %v", err)
	}
	_, other, _ := ed25519.GenerateKey(nil)
	if _, err := Build(plan, BuildOptions{Version: "1", Out: filepath.Join(root, "c"), Key: other}); err == nil || !strings.Contains(err.Error(), "isn't one of") {
		t.Errorf("wrong key: %v", err)
	}

	result, err := Build(plan, BuildOptions{Version: "1", Out: filepath.Join(root, "upload"), Key: key})
	if err != nil {
		t.Fatal(err)
	}

	profile, _ := spec.ParseProfile(readFile(t, filepath.Join(root, "profile.json")))
	keys, _ := profile.ManifestKeys()
	for _, b := range result.Built {
		data := readFile(t, b.Manifest)
		if err := spec.VerifyManifest(data, readFile(t, b.Manifest+".sig"), keys); err != nil {
			t.Errorf("%s: %v", b.Name, err)
		}
	}
}

func TestLoadKeyRejects(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "junk.pem", "not a key")
	if _, err := LoadKey(filepath.Join(dir, "junk.pem")); err == nil {
		t.Error("junk accepted as a key")
	}
}

func readFile(t *testing.T, file string) []byte {
	t.Helper()

	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	return data
}
