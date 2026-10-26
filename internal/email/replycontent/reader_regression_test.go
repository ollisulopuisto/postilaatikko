package replycontent

import (
	"strings"
	"testing"
)

func TestReaderSignatureAndUnprefixedHistory(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		html       bool
	}{
		{"plain em dash", "Here it is!\n\n—\nA favorite quotation\nme@example.com * +358 50 123 4567", false},
		{"html em dash", "<div>Here it is!</div><div><br></div><div>—</div><div>A favorite quotation</div><div>me@example.com * +358 50 123 4567</div>", true},
		{"plain history", "New answer\n\nOn 13. Sep 2025 at 14.51 -0500, Person <person@example.com>, wrote:\nOld answer", false},
		{"html history", "<div>New answer</div><div><br></div><div>On 13. Sep 2025 at 14.51 -0500, Person &lt;person@example.com&gt;, wrote:</div><div>Old answer</div>", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var result Result
			if tc.html {
				result = ExtractHTML(tc.body)
			} else {
				result = ExtractPlain(tc.body)
			}
			var shown strings.Builder
			hidden := false
			for _, b := range result.Blocks {
				if b.Kind == BlockAuthored || b.Kind == BlockUnknown {
					shown.WriteString(b.Content)
				} else {
					hidden = true
				}
			}
			if !hidden || strings.Contains(shown.String(), "favorite quotation") || strings.Contains(shown.String(), "Old answer") {
				t.Fatalf("unstripped: %+v", result)
			}
		})
	}
}

func TestReaderKeepsMeaningfulDashesAndInlineAnswers(t *testing.T) {
	for _, body := range []string{
		"There are two choices.\n\n—\nChoose one today.",
		"On Mon, Jul 8, 2025, Person wrote:\n> First question\nMy first answer\n> Second question\nMy second answer",
		"Please contact support@example.com\n\n—\nThis is a separator, not a signature.",
	} {
		result := ExtractPlain(body)
		var shown strings.Builder
		for _, b := range result.Blocks {
			if b.Kind == BlockAuthored || b.Kind == BlockUnknown {
				shown.WriteString(b.Content)
			}
		}
		for _, line := range []string{"Choose one today.", "My first answer", "My second answer", "This is a separator"} {
			if strings.Contains(body, line) && !strings.Contains(shown.String(), line) {
				t.Fatalf("lost %q: %+v", line, result)
			}
		}
	}
}

func TestReaderHTMLSplittingPreservesMarkupAndImageBlocking(t *testing.T) {
	body := `<div>New <strong>answer</strong> <a href="https://example.com/item">link</a><img src="https://example.com/pixel.png"></div><div><br></div><div>—</div><div>me@example.com</div>`
	result := ExtractHTML(body)
	var authored strings.Builder
	for _, block := range result.Blocks {
		if block.Kind == BlockAuthored {
			authored.WriteString(block.Content)
		}
	}
	content := authored.String()
	for _, fragment := range []string{"<strong>answer</strong>", `href="https://example.com/item"`, "data-original-src="} {
		if !strings.Contains(content, fragment) {
			t.Fatalf("lost %q: %s", fragment, content)
		}
	}
	if strings.Contains(content, ` src="https://example.com/pixel.png"`) {
		t.Fatal("remote image became unblocked")
	}
	if strings.Contains(content, "me@example.com") {
		t.Fatal("signature not removed")
	}
}
