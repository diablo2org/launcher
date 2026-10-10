package spec

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
)

// Signing is the keys a server signs its manifests with. See docs/SPEC.md
// section 4.2.
type Signing struct {
	Keys []string `json:"keys"`
}

// ErrSignature means a manifest's signature is missing or isn't from one of
// the server's keys.
var ErrSignature = errors.New("manifest signature doesn't match the server's key")

// ManifestKeys returns the keys the profile's manifests must be signed with,
// or none when the server doesn't sign them.
func (p *Profile) ManifestKeys() ([]ed25519.PublicKey, error) {
	if p.Signing == nil {
		return nil, nil
	}

	keys := make([]ed25519.PublicKey, 0, len(p.Signing.Keys))
	for _, k := range p.Signing.Keys {
		key, err := ParsePublicKey(k)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}

	return keys, nil
}

// ParsePublicKey reads an Ed25519 public key as a profile gives it: 32 bytes
// in standard base64.
func ParsePublicKey(s string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.Strict().DecodeString(s)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%q is not an Ed25519 public key in base64", s)
	}

	return ed25519.PublicKey(raw), nil
}

// FormatPublicKey writes a public key as a profile gives it.
func FormatPublicKey(key ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(key)
}

// SignatureURL is where a manifest's signature is published: next to it,
// with ".sig" added to its name.
func SignatureURL(manifestURL string) (string, error) {
	u, err := url.Parse(manifestURL)
	if err != nil {
		return "", err
	}
	u.Path += ".sig"
	u.RawPath = ""

	return u.String(), nil
}

// SignManifest returns the signature file for a manifest's exact bytes: the
// Ed25519 signature in base64, on one line.
func SignManifest(data []byte, key ed25519.PrivateKey) []byte {
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, data)) + "\n")
}

// VerifyManifest checks a signature file against a manifest's exact bytes
// and the server's keys. Any one key will do.
func VerifyManifest(data, sigFile []byte, keys []ed25519.PublicKey) error {
	sig, err := base64.StdEncoding.Strict().DecodeString(string(bytes.TrimSpace(sigFile)))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("%w: not a signature", ErrSignature)
	}

	for _, k := range keys {
		if ed25519.Verify(k, data, sig) {
			return nil
		}
	}

	return ErrSignature
}
