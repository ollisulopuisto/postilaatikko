package app

import "testing"

func TestRemoteLaterFolderGuard(t *testing.T) {
	for _, folderType := range []string{"readLater", "replyLater"} {
		if !isRemoteUnifiedFolderType(folderType) {
			t.Errorf("remote guard rejects %s", folderType)
		}
	}
	if isRemoteUnifiedFolderType("arbitrary") {
		t.Fatal("remote guard accepts unknown view")
	}
}
