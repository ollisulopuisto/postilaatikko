package replycontent

import (
	"strings"
	"testing"
)

func TestOutlookQuotedHeadersAcrossHTMLLayouts(t *testing.T) {
	cases := map[string]string{
		"divs":              `<div>New answer</div><hr><div><b>From:</b> Example Sender &lt;sender@example.com&gt;</div><div><b>Date:</b> Sunday, 14 September 2025 at 23.22</div><div><b>To:</b> Peer &lt;peer@example.com&gt;</div><div><b>Subject:</b> Example topic</div><div>Old quoted answer</div>`,
		"breaks":            `<p>New answer</p><p><b>From:</b> Example Sender &lt;sender@example.com&gt;<br><b>Date:</b> Sunday, 14 September 2025 at 23.22<br><b>To:</b> Peer &lt;peer@example.com&gt;<br><b>Subject:</b> Example topic</p><p>Old quoted answer</p>`,
		"table":             `<p>New answer</p><table><tr><td><b>From:</b></td><td>Example Sender &lt;sender@example.com&gt;</td></tr><tr><td><b>Date:</b></td><td>Sunday, 14 September 2025 at 23.22</td></tr><tr><td><b>To:</b></td><td>Peer &lt;peer@example.com&gt;</td></tr><tr><td><b>Subject:</b></td><td>Example topic</td></tr></table><p>Old quoted answer</p>`,
		"named sender":      `<p>New answer</p><div>From: Example Sender</div><div>Date: 14 September 2025</div><div>To: Peer</div><div>Subject: Example topic</div><p>Old quoted answer</p>`,
		"nested paragraphs": `<p>New answer</p><div><p>From: Example Sender</p></div><div><p>Date: 14 September 2025</p></div><div><p>To: Peer</p></div><div><p>Subject: Example topic</p></div><p>Old quoted answer</p>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			result := ExtractHTML(body)
			var shown, quoted strings.Builder
			for _, b := range result.Blocks {
				if b.Kind == BlockQuoted {
					quoted.WriteString(b.Content)
				} else {
					shown.WriteString(b.Content)
				}
			}
			if !strings.Contains(shown.String(), "New answer") || strings.Contains(shown.String(), "Old quoted answer") || !strings.Contains(quoted.String(), "Old quoted answer") {
				t.Fatalf("bad split: %+v", result)
			}
			if err := result.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSpacedHeaderRunsAndOrdinaryFromText(t *testing.T) {
	for _, tc := range []struct {
		body   string
		quoted bool
	}{
		{"New answer\n\nFrom: Example Sender\n\nDate: Sunday\n\nTo: Peer\n\nSubject: Topic\n\nOld quoted answer", true},
		{"From: Example Sender\nThis is ordinary prose, not history.", false},
		{"New answer\n\nFrom: first\n\nFrom: second\n\nFrom: third\nOrdinary list", false},
	} {
		result := ExtractPlain(tc.body)
		hasQuote := false
		for _, b := range result.Blocks {
			hasQuote = hasQuote || b.Kind == BlockQuoted
		}
		if hasQuote != tc.quoted {
			t.Fatalf("unexpected header recognition: %+v", result)
		}
	}
}

func TestHeaderExamplesInsideCodeRemainVisible(t *testing.T) {
	body := `<p>This is an example, not a quoted message:</p><pre><code>From: sender@example.com
Date: Sunday
To: peer@example.com
Subject: Demonstration
Code example content</code></pre>`
	result := ExtractHTML(body)
	for _, block := range result.Blocks {
		if block.Kind == BlockQuoted {
			t.Fatalf("code example hidden: %+v", result)
		}
	}
}
