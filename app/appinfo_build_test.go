package app

import "testing"

// The About dialog must distinguish individual builds: two DMGs reporting the
// same VERSION are indistinguishable when triaging bug reports (the
// 2026-11-19 search-bindings confusion, where a pre-fix binary reported the
// same version as the fixed one). Build is injected at link time.
func TestGetAppInfoReportsBuildIdentifier(t *testing.T) {
	defer func(prev string) { Build = prev }(Build)
	Build = "8c95950e-test.44518590233.1"
	info := (&App{}).GetAppInfo()
	if info.Build != Build {
		t.Fatalf("AppInfo.Build = %q, want %q", info.Build, Build)
	}
}

// A plain `go build` without the ldflags injection leaves Build empty; the
// About dialog hides the build line rather than showing a placeholder.
func TestGetAppInfoBuildDefaultsEmpty(t *testing.T) {
	defer func(prev string) { Build = prev }(Build)
	Build = ""
	info := (&App{}).GetAppInfo()
	if info.Build != "" {
		t.Fatalf("AppInfo.Build = %q, want empty without ldflags injection", info.Build)
	}
}
