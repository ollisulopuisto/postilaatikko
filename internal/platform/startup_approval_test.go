package platform

import (
	"encoding/binary"
	"testing"
)

func TestWindowsStartupApprovalPreservesDisabledAndUnknownStates(t *testing.T) {
	for _, state := range []uint32{0, 1, 2, 3, 6, 7, 99} {
		data := make([]byte, 12)
		binary.LittleEndian.PutUint32(data, state)
		enabled, err := windowsStartupApprovalEnabled(data)
		if err != nil || enabled != (state == 2 || state == 6) {
			t.Fatalf("state %d enabled=%v err=%v", state, enabled, err)
		}
	}
	for _, size := range []int{0, 1, 4, 11, 13} {
		if enabled, err := windowsStartupApprovalEnabled(make([]byte, size)); err == nil || enabled {
			t.Fatalf("malformed approval %d accepted", size)
		}
	}
}
