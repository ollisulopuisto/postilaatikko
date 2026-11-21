package email

import (
	"github.com/ollisulopuisto/postilaatikko/internal/message"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAllSameNamePartsNeverOverwrite(t *testing.T) {
	dir := t.TempDir()
	downloader := NewAttachmentDownloader("")
	att := &message.Attachment{Filename: "same.png"}
	one, err := downloader.SaveAttachmentToDirectory(att, []byte("one"), dir)
	if err != nil {
		t.Fatal(err)
	}
	two, err := downloader.SaveAttachmentToDirectory(att, []byte("two"), dir)
	if err != nil {
		t.Fatal(err)
	}
	if one == two || filepath.Base(two) != "same (2).png" {
		t.Fatal("same-name exports collide")
	}
	for path, want := range map[string]string{one: "one", two: "two"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatal("export lost real part")
		}
	}
}

func TestRawFallbackRejectsAmbiguousFilename(t *testing.T) {
	d := NewAttachmentDownloader("")
	if bytes, err := d.ExtractUniqueAttachmentContent([]byte(repairFixture()), "same.png"); err == nil {
		t.Fatal("duplicate filename returned first payload", string(bytes))
	}
	raw := strings.Replace(repairFixture(), "attachment; filename=same.png", "attachment; filename=other.png", 1)
	for name, want := range map[string]string{"same.png": "one", "other.png": "two"} {
		got, err := d.ExtractUniqueAttachmentContent([]byte(raw), name)
		if err != nil || string(got) != want {
			t.Fatal("unique part missing", name, string(got), err)
		}
	}
	if _, err := d.ExtractUniqueAttachmentContent([]byte(strings.TrimSuffix(raw, "--b--\r\n")), "same.png"); err == nil {
		t.Fatal("incomplete MIME established uniqueness")
	}
}

func TestUniqueAttachmentFallbackIgnoresUnnamedBodyAndSignatures(t *testing.T) {
	d := NewAttachmentDownloader("")
	for _, bodyType := range []string{"text/plain", "text/html", "application/pkcs7-signature", "application/x-pkcs7-signature", "application/pgp-signature"} {
		raw := "MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\nContent-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=attachment.bin\r\n\r\nfile bytes\r\n--b\r\nContent-Type: " + bodyType + "\r\n\r\nnot a file\r\n--b--\r\n"
		got, err := d.ExtractUniqueAttachmentContent([]byte(raw), "attachment.bin")
		if err != nil || string(got) != "file bytes" {
			t.Fatal("nonattachment blocked unique file", bodyType, string(got), err)
		}
	}
}

func TestInlineTextAttachmentFallbackKeepsCalendarAndContact(t *testing.T) {
	for _, ct := range []string{"text/calendar", "text/vcard"} {
		prefix := "Content-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\nContent-Type: " + ct + "\r\nContent-Disposition: inline; filename=file.txt\r\n\r\ninline bytes\r\n"
		raw := prefix + "--b--\r\n"
		got, err := NewAttachmentDownloader("").ExtractUniqueAttachmentContent([]byte(raw), "file.txt")
		if err != nil || string(got) != "inline bytes" {
			t.Fatal("listed inline text file rejected", ct, string(got), err)
		}
		raw = prefix + "--b\r\nContent-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=file.txt\r\n\r\nother bytes\r\n--b--\r\n"
		if got, err := NewAttachmentDownloader("").ExtractUniqueAttachmentContent([]byte(raw), "file.txt"); err == nil {
			t.Fatal("inline text hidden from ambiguity check", ct, string(got))
		}
	}
}

func TestNamedGmailTextWithoutDispositionIsAFile(t *testing.T) {
	raw := "Content-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\nContent-Type: text/plain; name=notes.txt\r\n\r\nnotes\r\n--b\r\nContent-Type: text/plain\r\n\r\nbody\r\n--b--\r\n"
	got, err := NewAttachmentDownloader("").ExtractUniqueAttachmentContent([]byte(raw), "notes.txt")
	if err != nil || string(got) != "notes" {
		t.Fatal("named Gmail text rejected", string(got), err)
	}
}
