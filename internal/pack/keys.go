package pack

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/diablo2org/launcher/internal/spec"
)

// GenerateKey makes a manifest signing key. It returns the private key as a
// PEM file to keep secret, and the public key as the profile's signing.keys
// gives it.
func GenerateKey() (private []byte, public string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", err
	}

	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, "", err
	}

	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), spec.FormatPublicKey(pub), nil
}

// LoadKey reads a signing key written by GenerateKey.
func LoadKey(file string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("%s is not a private key file from d2pack keygen", file)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%s is not an Ed25519 key", file)
	}

	return priv, nil
}

// checkKey makes sure manifests are signed exactly when the profile asks: a
// profile with keys needs a key that is one of them, and one without has
// nothing to check a signature against.
func checkKey(profile *spec.Profile, key ed25519.PrivateKey) error {
	keys, err := profile.ManifestKeys()
	if err != nil {
		return err
	}

	switch {
	case keys == nil && key == nil:
		return nil
	case keys == nil:
		return fmt.Errorf("the profile has no signing keys, so the launcher wouldn't check this signature; add %s to its signing.keys first", spec.FormatPublicKey(key.Public().(ed25519.PublicKey)))
	case key == nil:
		return errors.New("the profile's manifests must be signed: pass -key with the key file from d2pack keygen")
	}

	pub := key.Public().(ed25519.PublicKey)
	if !slices.ContainsFunc(keys, func(k ed25519.PublicKey) bool { return pub.Equal(k) }) {
		return fmt.Errorf("the key's public key %s isn't one of the profile's signing.keys", spec.FormatPublicKey(pub))
	}

	return nil
}

// SignManifestFile writes a manifest's signature beside it, as
// <manifest>.sig.
func SignManifestFile(file string, key ed25519.PrivateKey) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	return os.WriteFile(file+".sig", spec.SignManifest(data, key), 0o644)
}
