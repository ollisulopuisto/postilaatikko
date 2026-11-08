package platform

import (
	"net/url"
	"strings"
)

// DialogIcon controls the icon and severity styling of native OS dialogs
// shown via ShowDialog / ShowDialogAsync.
type DialogIcon int

const (
	// DialogIconError — red X / stop icon. Use for unrecoverable failures.
	DialogIconError DialogIcon = iota
	// DialogIconWarning — yellow triangle / caution icon. Use for recoverable
	// issues the user should know about.
	DialogIconWarning
	// DialogIconInfo — neutral info icon. Use for purely informational messages.
	DialogIconInfo
)

// ShowDialog displays a native OS dialog with a single OK button and blocks
// until the user dismisses it. Use for pre-Wails startup failures where there
// is no Svelte UI to render the error in, and the caller intends to exit
// immediately after the dialog closes.
//
// Linux: zenity → kdialog → xmessage → stderr fallback chain. macOS: osascript.
// Windows: user32!MessageBoxW. Display failures degrade silently to a stderr
// write so the underlying message is still surfaced somewhere.
//
// Does NOT call os.Exit — caller's responsibility.
func ShowDialog(icon DialogIcon, title, text string) {
	showDialog(icon, title, text, true)
}

// ShowDialogAsync is the non-blocking variant of ShowDialog. Use when the
// dialog is informational and the program should keep running while the user
// reads (e.g. background warnings detected mid-session).
func ShowDialogAsync(icon DialogIcon, title, text string) {
	showDialog(icon, title, text, false)
}

// ShowDialogWithLink displays a blocking native dialog with two buttons: a
// default close button ("Quit") and an action button labeled actionLabel
// that opens actionURL via the OS's default URL handler when clicked.
// Returns after the user dismisses the dialog. If the user clicked the
// action button, the URL is opened before this function returns.
//
// Use this for startup failures that have an associated docs URL — e.g.,
// the schema-too-new error pointing at docs/SQL_ROLLBACK.md on GitHub.
//
// Backend behavior:
//   - Linux (zenity): two-button dialog via --extra-button; URLs inline
//     in text are also clickable via Pango markup.
//   - Linux (kdialog / xmessage): two-button dialog with custom labels.
//   - macOS (osascript): two-button "display dialog".
//   - Windows: MB_YESNO with explanatory text appended.
//
// All backends degrade gracefully — if the chosen tool isn't available,
// the dialog falls back to a single-button display with actionURL
// included in the body as copyable plain text.
func ShowDialogWithLink(icon DialogIcon, title, text, actionLabel, actionURL string) {
	if !ValidateDialogURL(actionURL) {
		// If the action URL fails security validation (e.g. contains flags, control characters,
		// or dangerous schemes), degrade safely to a single-button dialog without URL opening.
		ShowDialog(icon, title, text)
		return
	}
	showDialogWithLink(icon, title, text, actionLabel, actionURL)
}

// ValidateDialogURL validates that a URL passed to native dialog link openers
// uses an approved protocol (http, https, mailto) and does not contain control
// characters or leading dashes that could trigger CLI option injection in
// system handlers (e.g. xdg-open or open).
func ValidateDialogURL(rawURL string) bool {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" || strings.ContainsAny(trimmed, "\r\n\x00") {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
	}

	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "http", "https":
		host := parsed.Hostname()
		return host != "" && !strings.HasPrefix(host, "-")
	case "mailto":
		target := parsed.Opaque
		if target == "" {
			target = parsed.Path
		}
		if strings.HasPrefix(target, "-") {
			return false
		}
		// Also inspect unescaped target to prevent percent-encoded dashes (%2D) from bypassing CLI option guards
		if unescaped, err := url.PathUnescape(target); err == nil && strings.HasPrefix(unescaped, "-") {
			return false
		}
		return target != "" || parsed.RawQuery != ""
	default:
		return false
	}
}
