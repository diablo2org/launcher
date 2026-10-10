package spec

import (
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"
)

func TestManifestSignature(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	otherPub, otherPriv, _ := ed25519.GenerateKey(nil)
	manifest := []byte(`{"schema":1,"server":"slashdiablo","version":"1","files":[]}`)

	p, err := ParseProfile(edit(t, func(d map[string]interface{}) {
		d["signing"] = map[string]interface{}{"keys": []interface{}{FormatPublicKey(otherPub), FormatPublicKey(pub)}}
	}))
	if err != nil {
		t.Fatal(err)
	}
	keys, err := p.ManifestKeys()
	if err != nil || len(keys) != 2 {
		t.Fatalf("keys = %v, %v", keys, err)
	}

	// Either key will do, so a server can move to a new one.
	for _, k := range []ed25519.PrivateKey{priv, otherPriv} {
		if err := VerifyManifest(manifest, SignManifest(manifest, k), keys); err != nil {
			t.Error(err)
		}
	}

	_, stranger, _ := ed25519.GenerateKey(nil)
	tampered := []byte(strings.Replace(string(manifest), `"1"`, `"2"`, 1))
	for name, tt := range map[string]struct{ data, sig []byte }{
		"changed manifest": {tampered, SignManifest(manifest, priv)},
		"another key":      {manifest, SignManifest(manifest, stranger)},
		"empty":            {manifest, nil},
		"not base64":       {manifest, []byte("not a signature")},
		"short":            {manifest, []byte("AAAA")},
	} {
		if err := VerifyManifest(tt.data, tt.sig, keys); !errors.Is(err, ErrSignature) {
			t.Errorf("%s: %v", name, err)
		}
	}

	// A profile without signing has no keys.
	if keys, err := exampleProfile(t).ManifestKeys(); keys != nil || err != nil {
		t.Errorf("unsigned profile: %v, %v", keys, err)
	}

	for _, keys := range [][]interface{}{
		{"not a key"},
		{FormatPublicKey(pub)[:40]},
		{},
	} {
		if _, err := ParseProfile(edit(t, func(d map[string]interface{}) { d["signing"] = map[string]interface{}{"keys": keys} })); err == nil {
			t.Errorf("keys %v accepted", keys)
		}
	}
}

func TestSignatureURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://files.example.net/live/manifest.json":     "https://files.example.net/live/manifest.json.sig",
		"https://files.example.net/live/manifest.json?v=2": "https://files.example.net/live/manifest.json.sig?v=2",
		"https://files.example.net/live%20x/manifest.json": "https://files.example.net/live%20x/manifest.json.sig",
	} {
		if got, err := SignatureURL(in); err != nil || got != want {
			t.Errorf("SignatureURL(%s) = %s, %v; want %s", in, got, err, want)
		}
	}
}
