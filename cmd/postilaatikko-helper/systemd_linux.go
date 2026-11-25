//go:build linux

package main

import "strings"

// renderSystemdUnit renders a user unit without invoking a shell. args must
// contain the serve subcommand and its flags, as in the launchd ProgramArguments.
// Both systemd specifiers (%) and environment expansion ($) are disabled for
// literal user-supplied paths and arguments.
func renderSystemdUnit(binary string, args []string) string {
	var unit strings.Builder
	unit.WriteString("[Unit]\nDescription=Postilaatikko helper\n\n[Service]\nType=simple\nExecStart=")
	unit.WriteString(quoteSystemdArg(binary))
	for _, arg := range args {
		unit.WriteByte(' ')
		unit.WriteString(quoteSystemdArg(arg))
	}
	unit.WriteString("\nRestart=on-failure\nRestartSec=5\n\n[Install]\nWantedBy=default.target\n")
	return unit.String()
}

func quoteSystemdArg(arg string) string {
	var quoted strings.Builder
	quoted.WriteByte('"')
	for _, char := range arg {
		switch char {
		case '\\':
			quoted.WriteString(`\\`)
		case '"':
			quoted.WriteString(`\"`)
		case '%':
			quoted.WriteString("%%")
		case '$':
			quoted.WriteString("$$")
		case '\n':
			quoted.WriteString(`\n`)
		case '\r':
			quoted.WriteString(`\r`)
		case '\t':
			quoted.WriteString(`\t`)
		default:
			if char < 0x20 || char == 0x7f {
				const hex = "0123456789abcdef"
				quoted.WriteString(`\x`)
				quoted.WriteByte(hex[byte(char)>>4])
				quoted.WriteByte(hex[byte(char)&15])
			} else {
				quoted.WriteRune(char)
			}
		}
	}
	quoted.WriteByte('"')
	return quoted.String()
}
