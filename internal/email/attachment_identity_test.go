package email

import "testing"

func TestExtractAttachmentsRepeatKeepsDistinctParts(t *testing.T) {
	raw := []byte("MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\nContent-Type: image/png\r\nContent-Disposition: attachment; filename=image.png\r\n\r\none\r\n--b\r\nContent-Type: image/png\r\nContent-Disposition: attachment; filename=image.png\r\n\r\ntwo\r\n--b--\r\n")
	extractor := NewAttachmentExtractor()
	first, err := extractor.ExtractAttachments("mail-a", raw)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := extractor.ExtractAttachments("mail-a", raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || len(repeated) != 2 {
		t.Fatal("lost real MIME parts")
	}
	if first[0].Attachment.ID != repeated[0].Attachment.ID || first[1].Attachment.ID != repeated[1].Attachment.ID {
		t.Fatal("reparse changes attachment identity")
	}
	if first[0].Attachment.ID == first[1].Attachment.ID {
		t.Fatal("collapsed distinct same-name parts")
	}
	if string(first[0].Content) != "one" || string(first[1].Content) != "two" {
		t.Fatal("part payloads changed")
	}
}
