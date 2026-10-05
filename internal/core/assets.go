package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/diablo2org/launcher/internal/spec"
)

// Size limits for branding images, from docs/SPEC.md section 3.
var assetLimits = map[string]int{
	"logo":       512 << 10,
	"background": 3 << 20,
}

// Asset returns one of a server's branding images, cached after the first
// download and checked against its SHA-256 when the profile gives one.
func (m *Manager) Asset(ctx context.Context, id, which string) ([]byte, error) {
	p, err := m.Profile(ctx, id)
	if err != nil {
		return nil, err
	}

	var a *spec.Asset
	switch which {
	case "logo":
		a = p.Branding.Logo
	case "background":
		a = p.Branding.Background
	default:
		return nil, fmt.Errorf("unknown asset %q", which)
	}
	if a == nil {
		return nil, fmt.Errorf("%s has no %s", p.Name, which)
	}

	name := cacheName(which, a.URL+"#"+a.SHA256)
	if data, _ := m.store.ReadCache(id, name); data != nil {
		return data, nil
	}

	data, err := m.client(p.Hosts).Document(ctx, a.URL)
	if err != nil {
		return nil, err
	}

	if len(data) > assetLimits[which] {
		return nil, fmt.Errorf("%s %s is larger than %d KB", p.Name, which, assetLimits[which]>>10)
	}

	if a.SHA256 != "" {
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != a.SHA256 {
			return nil, fmt.Errorf("%s %s does not match its SHA-256", p.Name, which)
		}
	}

	if err := m.store.WriteCache(id, name, data); err != nil {
		return nil, err
	}

	return data, nil
}
