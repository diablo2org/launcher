package launch

import (
	"errors"
	"os/exec"

	"golang.org/x/sys/windows/registry"
)

const (
	battleNetKey = `Software\Battle.net\Configuration`
	gatewayValue = "Diablo II Battle.net Gateways"
	diabloKey    = `Software\Blizzard Entertainment\Diablo II`
)

// SystemRegistry is the current user's real registry.
type SystemRegistry struct{}

// GatewayList reads the existing gateway list; none yet is an empty list.
func (SystemRegistry) GatewayList() ([]string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, battleNetKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer k.Close()

	values, _, err := k.GetStringsValue(gatewayValue)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}

	return values, err
}

// SetGatewayList writes the gateway list.
func (SystemRegistry) SetGatewayList(values []string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, battleNetKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	return k.SetStringsValue(gatewayValue, values)
}

// SetString writes a value under the game's own key.
func (SystemRegistry) SetString(name, value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, diabloKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	return k.SetStringValue(name, value)
}

// StartProcess starts a box and lets it run on its own.
func StartProcess(b Box) error {
	cmd := exec.Command(b.Exe, b.Args...)
	cmd.Dir = b.Dir

	if err := cmd.Start(); err != nil {
		return err
	}

	return cmd.Process.Release()
}
