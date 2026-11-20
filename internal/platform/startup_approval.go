package platform

import (
	"encoding/binary"
	"fmt"
)

// windowsStartupApprovalEnabled treats unknown or malformed approval data as
// disabled. Missing values are handled by the registry caller, not this parser.
func windowsStartupApprovalEnabled(data []byte) (bool, error) {
	if len(data) != 12 {
		return false, fmt.Errorf("invalid Windows startup approval length: %d", len(data))
	}
	state := binary.LittleEndian.Uint32(data[:4])
	return state == 2 || state == 6, nil
}
