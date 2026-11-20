package main

import "testing"

func TestPostilaatikkoDebugAlias(t *testing.T) {
	t.Setenv("AERION_DEBUG", "")
	t.Setenv("POSTILAATIKKO_DEBUG", "1")
	if !DebugMode() {
		t.Fatal("new debug name ignored")
	}
}
