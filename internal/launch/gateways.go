// Package launch starts the game for a server: it points Battle.net at the
// server's gateway, then starts each box from the server's folder.
package launch

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/d2org/launcher/internal/spec"
)

// DefaultGatewayHeader is written when there's no existing list to keep the
// header from. It is the value PvPGN's own gateway installer writes.
const DefaultGatewayHeader = "1001"

// GatewayList is the game's "Diablo II Battle.net Gateways" registry value:
// a header, the selected gateway as a 1-based two digit index, then a host,
// timezone and name for each gateway.
type GatewayList struct {
	Header   string
	Selected int
	Gateways []spec.Gateway
}

// ParseGatewayList reads the multi-string registry value. A malformed or
// empty value gives an empty list rather than an error, since the launcher
// is about to write a good one.
func ParseGatewayList(values []string) GatewayList {
	l := GatewayList{Header: DefaultGatewayHeader}

	if len(values) < 2 {
		return l
	}

	if strings.TrimSpace(values[0]) != "" {
		l.Header = strings.TrimSpace(values[0])
	}
	l.Selected, _ = strconv.Atoi(strings.TrimSpace(values[1]))

	for i := 2; i+2 < len(values); i += 3 {
		tz, _ := strconv.Atoi(strings.TrimSpace(values[i+1]))
		l.Gateways = append(l.Gateways, spec.Gateway{
			Host:     strings.TrimSpace(values[i]),
			Timezone: tz,
			Name:     strings.TrimSpace(values[i+2]),
		})
	}

	return l
}

// Values renders the list as the registry's multi-string.
func (l GatewayList) Values() []string {
	out := []string{l.Header, fmt.Sprintf("%02d", l.Selected)}
	for _, g := range l.Gateways {
		out = append(out, g.Host, strconv.Itoa(g.Timezone), g.Name)
	}

	return out
}

// Merge adds a server's gateways to the player's existing list and selects
// the server's first one. Gateways the player added themselves are kept; an
// existing entry for the same host is replaced so it isn't listed twice.
func Merge(existing GatewayList, server []spec.Gateway) GatewayList {
	ours := make(map[string]bool, len(server))
	for _, g := range server {
		ours[strings.ToLower(g.Host)] = true
	}

	merged := GatewayList{Header: existing.Header}
	if merged.Header == "" {
		merged.Header = DefaultGatewayHeader
	}

	for _, g := range existing.Gateways {
		if !ours[strings.ToLower(g.Host)] {
			merged.Gateways = append(merged.Gateways, g)
		}
	}

	merged.Selected = len(merged.Gateways) + 1
	merged.Gateways = append(merged.Gateways, server...)

	return merged
}
