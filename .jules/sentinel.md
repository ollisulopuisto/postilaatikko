# Sentinel Security Journal

## 2026-10-31 - URL Scheme Validation and Fallback Bypasses
**Vulnerability:** External URL opening in `App.OpenURL` relied on naive string prefix matching (`allowedPrefixes = []string{"http://", "https://", "mailto:"}`), while frontend `EmailBody.svelte` caught failures from `OpenURL` and fell back to unvalidated `BrowserOpenURL(url)`.
**Learning:** Naive prefix checks can fail on case sensitivity (`HTTP://`), malformed URI structures, or scheme confusion, while UI fallback handlers can inadvertently bypass backend security controls when errors occur.
**Prevention:** Always parse URLs with standard libraries (`net/url.Parse`), validate scheme and host strictly, and never provide permissive client-side fallbacks that bypass centralized backend security policies.

## 2026-11-01 - Frontend URL Validation Parity and CLI Argument Injection Guard
**Vulnerability:** Frontend link validation (`isAllowedEmailURL`) used simple protocol matching (`new URL(val).protocol`), permitting malformed URLs without hostnames, URLs with control characters, and leading dashes in hostnames that could trigger option/flag injection when passed to URL openers.
**Learning:** In desktop applications wrapping OS URL openers (`xdg-open` or `open`), validating protocol alone is insufficient; hostnames must be non-empty and must not start with dash (`-`) characters to prevent CLI argument confusion.
**Prevention:** Enforce defense-in-depth on the frontend matching backend URL security constraints, ensuring non-empty hostnames and disallowing leading dashes and control characters.
