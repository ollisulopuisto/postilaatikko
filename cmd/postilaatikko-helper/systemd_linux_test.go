//go:build linux

package main

import (
	"os"
	"strings"
	"testing"
)

func TestRenderSystemdUnitGolden(t *testing.T) {
	args := []string{"serve", "--identity", "/home/user name/identity.json", "--data", "/home/user name/mail", "--queue", "/home/user name/queue", "--listen", "127.0.0.1:8026", "--api-listen", "127.0.0.1:8027", "--sync", "--allowed-host", "mail.example.test"}
	want, err := os.ReadFile("testdata/systemd-unit.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got := renderSystemdUnit("/opt/Postilaatikko Helper/helper", args); got != string(want) {
		t.Fatalf("unit mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestSystemdUnitEscapesArguments(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", `""`},
		{`a"b\c`, `"a\"b\\c"`},
		{"100% $HOME ${HOME}", `"100%% $$HOME $${HOME}"`},
		{"one\ntwo\rthree\tfour", `"one\ntwo\rthree\tfour"`},
		{"Mäkelä 日本", `"Mäkelä 日本"`},
	}
	for _, tt := range tests {
		if got := quoteSystemdArg(tt.in); got != tt.want {
			t.Errorf("quote(%q)=%q,want%q", tt.in, got, tt.want)
		}
	}
	// Injection-looking arguments stay inside ExecStart, never become new unit directives.
	unit := renderSystemdUnit("/bin/helper", []string{"serve", "x\n[Service]\nExecStart=/bin/false"})
	for _, line := range strings.Split(unit, "\n") {
		if line == "ExecStart=/bin/false" {
			t.Fatal("argument injected a directive")
		}
	}
}
