//go:build !windows

package launch

import "errors"

var errUnsupported = errors.New("launching is only supported on Windows for now")

// SystemRegistry has nothing to write to outside Windows yet. A Linux build
// will write into the Wine prefix instead.
type SystemRegistry struct{}

func (SystemRegistry) GatewayList() ([]string, error) { return nil, errUnsupported }
func (SystemRegistry) SetGatewayList([]string) error  { return errUnsupported }
func (SystemRegistry) String(string) (string, error)  { return "", errUnsupported }
func (SystemRegistry) SetString(string, string) error { return errUnsupported }

// StartProcess is not supported outside Windows yet.
func StartProcess(Box) error { return errUnsupported }
