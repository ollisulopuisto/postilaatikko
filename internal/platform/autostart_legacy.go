package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

var legacyDesktopIDs = []string{"io.github.wesleiaqui.eternomail", "io.github.hkdb.EternoMail", "aerion"}

func hasLegacyAutostart(dir, suffix string) bool {
	for _, id := range legacyDesktopIDs {
		info, err := os.Lstat(filepath.Join(dir, id+suffix))
		if err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}
func removeLegacyAutostart(dir, suffix string) error {
	for _, id := range legacyDesktopIDs {
		path := filepath.Join(dir, id+suffix)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove legacy login entry: %w", err)
		}
	}
	return nil
}
