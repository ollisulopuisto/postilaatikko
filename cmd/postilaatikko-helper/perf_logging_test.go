package main

import "testing"

func TestOptedInAPITimingIsNotSuppressedByWarnLevel(t *testing.T) {
	t.Setenv("ETERNOMAIL_API_TIMING", "")
	if got := helperLogLevel(); got != "warn" {
		t.Fatal(got)
	}
	t.Setenv("ETERNOMAIL_API_TIMING", "1")
	if got := helperLogLevel(); got != "info" {
		t.Fatal(got)
	}
}
