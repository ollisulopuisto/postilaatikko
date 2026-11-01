# Sentinel Security Journal

## 2026-10-31 - URL Scheme Validation and Fallback Bypasses
**Vulnerability:** External URL opening in `App.OpenURL` relied on naive string prefix matching (`allowedPrefixes = []string{"http://", "https://", "mailto:"}`), while frontend `EmailBody.svelte` caught failures from `OpenURL` and fell back to unvalidated `BrowserOpenURL(url)`.
**Learning:** Naive prefix checks can fail on case sensitivity (`HTTP://`), malformed URI structures, or scheme confusion, while UI fallback handlers can inadvertently bypass backend security controls when errors occur.
**Prevention:** Always parse URLs with standard libraries (`net/url.Parse`), validate scheme and host strictly, and never provide permissive client-side fallbacks that bypass centralized backend security policies.
