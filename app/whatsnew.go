package app

// RequestWhatsNewFromMenu routes the native Help > What's New item to the
// frontend, which opens the release-notes dialog with the full released
// history. It does not change the last-acknowledged version marker.
func (a *App) RequestWhatsNewFromMenu() {
	a.emitRuntimeEvent("whatsnew:requested")
}
